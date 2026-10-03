// Package seed fills an empty database with a demo: a user for each role,
// a project with flags in different states, a change request waiting for
// review, and two days of exposures.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/0mjs/flagpole/api/internal/flags"
	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/rules"
	"github.com/0mjs/flagpole/api/internal/store"
)

// Password is every demo user's password.
const Password = "flagpole-demo-password"

type Result struct {
	Users  []identity.User
	SDKKey string
	// DemoKey is for the demo shop, which reads web-app's development
	// environment, where changes apply without review.
	DemoKey string
}

func Run(ctx context.Context, db *postgres.DB, ids *identity.Service, prj *projects.Service, fl *flags.Service) (Result, error) {
	var res Result
	people := []struct {
		email, name string
		role        identity.Role
	}{
		{"ada@example.com", "Ada Lovelace", identity.Admin},
		{"linus@example.com", "Linus Approver", identity.Approver},
		{"grace@example.com", "Grace Hopper", identity.Editor},
		{"vera@example.com", "Vera Viewer", identity.Viewer},
	}
	users := map[identity.Role]identity.User{}
	hash, err := identity.HashPassword(Password)
	if err != nil {
		return res, err
	}
	for _, p := range people {
		row, err := db.CreateUser(ctx, store.CreateUserParams{Email: p.email, Name: p.name, Role: store.UserRole(p.role), PasswordHash: &hash})
		if err != nil {
			return res, fmt.Errorf("create %s (seed needs an empty database): %w", p.email, err)
		}
		u := identity.User{ID: row.ID, Email: row.Email, Name: row.Name, Role: p.role, Status: "active"}
		users[p.role] = u
		res.Users = append(res.Users, u)
	}
	admin, editor, approver := users[identity.Admin], users[identity.Editor], users[identity.Approver]

	detail, err := prj.Create(ctx, admin, "web-app", "Web app", "The storefront and checkout.")
	if err != nil {
		return res, err
	}
	p := detail.Project
	if _, err := prj.Create(ctx, admin, "mobile", "Mobile app", "iOS and Android."); err != nil {
		return res, err
	}

	str := func(v string) json.RawMessage { b, _ := json.Marshal(v); return b }
	newFlags := []flags.NewFlag{
		{Key: "new-checkout", Name: "New checkout", Description: "The one-page checkout.", Kind: "boolean", Tags: []string{"checkout"}},
		{Key: "button-color", Name: "Buy button color", Description: "A/B test of the buy button.", Kind: "string", Tags: []string{"experiment"},
			Variants: []flags.Variant{{Key: "blue", Value: str("blue")}, {Key: "green", Value: str("green")}, {Key: "orange", Value: str("orange")}}},
		{Key: "search-v2", Name: "Search v2", Description: "The rebuilt search service.", Kind: "boolean", Tags: []string{"search"}},
		{Key: "free-shipping-threshold", Name: "Free shipping threshold", Kind: "number", Tags: []string{"checkout", "pricing"},
			Variants: []flags.Variant{{Key: "standard", Value: json.RawMessage("50")}, {Key: "promo", Value: json.RawMessage("25")}}},
		{Key: "maintenance-banner", Name: "Maintenance banner", Kind: "boolean", Tags: []string{"ops"}},
	}
	for _, nf := range newFlags {
		if _, err := fl.Create(ctx, editor, p, nf); err != nil {
			return res, err
		}
	}

	pro := []rules.Condition{{Attribute: "plan", Operator: rules.In, Values: []string{"pro", "team"}}}
	set := func(flag, env string, cfg rules.Config) error {
		current, err := fl.Config(ctx, p, flag, env)
		if err != nil {
			return err
		}
		_, err = fl.UpdateConfig(ctx, editor, p, flag, env, cfg, current.Version)
		return err
	}
	steps := []struct {
		flag, env string
		cfg       rules.Config
	}{
		{"new-checkout", "development", rules.Config{Enabled: true, DefaultVariant: "on", OffVariant: "off", Rules: []rules.Rule{}}},
		{"new-checkout", "staging", rules.Config{Enabled: true, DefaultVariant: "off", OffVariant: "off", Rules: []rules.Rule{
			{Description: "Paying customers", Conditions: pro, Variant: "on"},
			{Description: "10% of everyone else", Rollout: []rules.Weight{{Variant: "on", Weight: 10}, {Variant: "off", Weight: 90}}},
		}}},
		{"button-color", "development", rules.Config{Enabled: true, DefaultVariant: "blue", OffVariant: "blue", Rules: []rules.Rule{
			{Rollout: []rules.Weight{{Variant: "blue", Weight: 34}, {Variant: "green", Weight: 33}, {Variant: "orange", Weight: 33}}},
		}}},
		{"search-v2", "development", rules.Config{Enabled: true, DefaultVariant: "on", OffVariant: "off", Rules: []rules.Rule{}}},
		{"search-v2", "staging", rules.Config{Enabled: true, DefaultVariant: "off", OffVariant: "off", Rules: []rules.Rule{
			{Description: "Internal staff", Conditions: []rules.Condition{{Attribute: "email", Operator: rules.EndsWith, Values: []string{"@example.com"}}}, Variant: "on"},
		}}},
	}
	for _, st := range steps {
		if err := set(st.flag, st.env, st.cfg); err != nil {
			return res, err
		}
	}

	// Production requires approval: one change approved, one waiting.
	cr, err := fl.RequestChange(ctx, editor, p, "button-color", "production", rules.Config{Enabled: true, DefaultVariant: "blue", OffVariant: "blue", Rules: []rules.Rule{
		{Description: "Even three-way split", Rollout: []rules.Weight{{Variant: "blue", Weight: 34}, {Variant: "green", Weight: 33}, {Variant: "orange", Weight: 33}}},
	}}, 1, "Start the experiment in production.")
	if err != nil {
		return res, err
	}
	if _, err := fl.Approve(ctx, approver, p, cr.ID, "Looks good, ship it."); err != nil {
		return res, err
	}
	if _, err := fl.RequestChange(ctx, editor, p, "new-checkout", "production", rules.Config{Enabled: true, DefaultVariant: "off", OffVariant: "off", Rules: []rules.Rule{
		{Description: "Paying customers", Conditions: pro, Variant: "on"},
	}}, 1, "Staging has looked healthy for a week; turn it on for paying customers."); err != nil {
		return res, err
	}

	if _, res.SDKKey, err = prj.CreateSDKKey(ctx, admin, p, "production", "storefront"); err != nil {
		return res, err
	}
	if _, res.DemoKey, err = prj.CreateSDKKey(ctx, admin, p, "development", "demo-shop"); err != nil {
		return res, err
	}

	// Two days of exposures for the experiment, so the charts have a shape.
	env, err := prj.Environment(ctx, p.ID, "production")
	if err != nil {
		return res, err
	}
	flag, err := db.GetFlag(ctx, store.GetFlagParams{ProjectID: p.ID, Key: "button-color"})
	if err != nil {
		return res, err
	}
	now := time.Now().UTC().Truncate(time.Hour)
	for h := 47; h >= 0; h-- {
		at := now.Add(-time.Duration(h) * time.Hour)
		base := 300 + 200*float64((at.Hour()+16)%24)/23 // busier in the day
		for variant, share := range map[string]float64{"blue": 0.34, "green": 0.33, "orange": 0.33} {
			n := int64(base*share*(0.85+rand.Float64()*0.3)) + 1
			if err := db.AddExposures(ctx, store.AddExposuresParams{FlagID: flag.ID, EnvironmentID: env.ID, Variant: variant, Bucket: at, Count: n}); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}
