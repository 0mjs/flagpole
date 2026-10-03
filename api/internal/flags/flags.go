// Package flags manages feature flags: their variants, how each behaves per
// environment, and change requests for environments that require a second
// person's approval.
package flags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/httpx"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/rules"
	"github.com/0mjs/flagpole/api/internal/store"
)

// Kind is the type of value a flag serves.
type Kind string

func (Kind) Enum() []any { return []any{"boolean", "string", "number", "json"} }

type Variant struct {
	Key         string          `json:"key" validate:"required,max=40" example:"on"`
	Value       json.RawMessage `json:"value" doc:"A JSON value of the flag's kind."`
	Description string          `json:"description,omitempty" validate:"max=200"`
}

// EnvironmentConfig is how a flag behaves in one environment.
type EnvironmentConfig struct {
	Environment string `json:"environment" example:"production"`
	rules.Config
	Version   int       `json:"version" doc:"Send it back as base_version when changing the config."`
	UpdatedAt time.Time `json:"updated_at"`
}

// Flag is a flag with its variants and its config in every environment.
type Flag struct {
	ID           uuid.UUID           `json:"id"`
	Key          string              `json:"key" example:"new-checkout"`
	Name         string              `json:"name" example:"New checkout"`
	Description  string              `json:"description"`
	Kind         Kind                `json:"kind"`
	Tags         []string            `json:"tags"`
	Variants     []Variant           `json:"variants"`
	Environments []EnvironmentConfig `json:"environments"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	ArchivedAt   *time.Time          `json:"archived_at,omitempty"`
}

// Summary is a flag as the list shows it: on or off in each environment.
type Summary struct {
	Key          string               `json:"key"`
	Name         string               `json:"name"`
	Kind         Kind                 `json:"kind"`
	Tags         []string             `json:"tags"`
	Environments map[string]EnvStatus `json:"environments" doc:"By environment key."`
	UpdatedAt    time.Time            `json:"updated_at"`
	ArchivedAt   *time.Time           `json:"archived_at,omitempty"`
}

func (Summary) OpenAPIName() string { return "FlagSummary" }

type EnvStatus struct {
	Enabled bool `json:"enabled"`
	Rules   int  `json:"rules"`
	Version int  `json:"version"`
}

// Invalidator forgets the cached SDK snapshot of an environment, so the
// next read sees a change at once.
type Invalidator interface {
	Invalidate(ctx context.Context, environmentID uuid.UUID)
}

type Service struct {
	db       *postgres.DB
	projects *projects.Service
	cache    Invalidator
}

func NewService(db *postgres.DB, p *projects.Service, cache Invalidator) *Service {
	return &Service{db: db, projects: p, cache: cache}
}

var flagKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

// List returns the project's flags, archived ones too when asked.
func (s *Service) List(ctx context.Context, p projects.Project, includeArchived bool) ([]Summary, error) {
	rows, err := s.db.ListFlags(ctx, store.ListFlagsParams{ProjectID: p.ID, IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	configs, err := s.db.ListFlagConfigs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byFlag := map[uuid.UUID]map[string]EnvStatus{}
	for _, c := range configs {
		if byFlag[c.FlagID] == nil {
			byFlag[c.FlagID] = map[string]EnvStatus{}
		}
		var rs []rules.Rule
		_ = json.Unmarshal(c.Rules, &rs)
		byFlag[c.FlagID][c.EnvironmentKey] = EnvStatus{Enabled: c.Enabled, Rules: len(rs), Version: int(c.Version)}
	}
	out := make([]Summary, len(rows))
	for i, r := range rows {
		out[i] = Summary{Key: r.Key, Name: r.Name, Kind: Kind(r.Kind), Tags: r.Tags, Environments: byFlag[r.ID],
			UpdatedAt: r.UpdatedAt, ArchivedAt: r.ArchivedAt}
	}
	return out, nil
}

// Get returns one flag with its variants and configs.
func (s *Service) Get(ctx context.Context, p projects.Project, key string) (Flag, error) {
	row, err := s.flag(ctx, s.db.Queries, p, key)
	if err != nil {
		return Flag{}, err
	}
	return s.load(ctx, s.db.Queries, row)
}

type NewFlag struct {
	Key         string
	Name        string
	Description string
	Kind        Kind
	Tags        []string
	Variants    []Variant
}

// Create makes a flag, off in every environment. A boolean flag gets the
// variants "on" and "off"; other kinds bring their own, the first being what
// it serves when off.
func (s *Service) Create(ctx context.Context, actor identity.User, p projects.Project, in NewFlag) (Flag, error) {
	problems := httpx.FieldErrors{}
	if !flagKeyPattern.MatchString(in.Key) {
		problems["key"] = "must start with a lowercase letter or digit, then up to 79 of those or . _ -"
	}
	variants, defaultVariant, offVariant := in.Variants, "", ""
	if in.Kind == "boolean" {
		if len(variants) > 0 {
			problems["variants"] = `boolean flags always have the variants "on" and "off"`
		}
		variants = []Variant{
			{Key: "on", Value: json.RawMessage("true")},
			{Key: "off", Value: json.RawMessage("false")},
		}
		defaultVariant, offVariant = "on", "off"
	} else {
		if len(variants) < 2 {
			problems["variants"] = "a " + string(in.Kind) + " flag needs at least two variants"
		}
		seen := map[string]bool{}
		for i, v := range variants {
			if seen[v.Key] {
				problems[fmt.Sprintf("variants[%d].key", i)] = "repeats " + v.Key
			}
			seen[v.Key] = true
			if msg := checkValue(in.Kind, v.Value); msg != "" {
				problems[fmt.Sprintf("variants[%d].value", i)] = msg
			}
		}
		if len(variants) > 0 {
			defaultVariant, offVariant = variants[0].Key, variants[0].Key
		}
	}
	if err := httpx.Invalid(problems); err != nil {
		return Flag{}, err
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}

	var flag Flag
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := q.CreateFlag(ctx, store.CreateFlagParams{
			ProjectID: p.ID, Key: in.Key, Name: in.Name, Description: in.Description,
			Kind: store.FlagKind(in.Kind), Tags: in.Tags, CreatedBy: &actor.ID,
		})
		if postgres.IsUniqueViolation(err) {
			return httpx.Conflict("a flag with key %s already exists in %s", in.Key, p.Name)
		}
		if err != nil {
			return err
		}
		for i, v := range variants {
			if _, err := q.CreateVariant(ctx, store.CreateVariantParams{
				FlagID: row.ID, Key: v.Key, Value: v.Value, Description: v.Description, Position: int32(i),
			}); err != nil {
				return err
			}
		}
		envs, err := q.ListEnvironments(ctx, p.ID)
		if err != nil {
			return err
		}
		for _, env := range envs {
			if _, err := q.CreateFlagConfig(ctx, store.CreateFlagConfigParams{
				FlagID: row.ID, EnvironmentID: env.ID, DefaultVariant: defaultVariant, OffVariant: offVariant,
				Rules: json.RawMessage("[]"), UpdatedBy: &actor.ID,
			}); err != nil {
				return err
			}
			if err := s.changed(ctx, q, p, env.ID, row.Key, 1); err != nil {
				return err
			}
		}
		if err := events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "flag.created", ProjectID: &p.ID, EntityType: "flag", EntityID: &row.ID,
			Summary: fmt.Sprintf("%s created flag %s", actor.Name, row.Key),
		}); err != nil {
			return err
		}
		flag, err = s.load(ctx, q, row)
		return err
	})
	if err == nil {
		s.invalidateAll(ctx, flag)
	}
	return flag, err
}

// Update changes a flag's name, description or tags; nil leaves one as is.
func (s *Service) Update(ctx context.Context, actor identity.User, p projects.Project, key string, name, description *string, tags []string) (Flag, error) {
	var flag Flag
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := s.flag(ctx, q, p, key)
		if err != nil {
			return err
		}
		params := store.UpdateFlagParams{ID: row.ID, Name: row.Name, Description: row.Description, Tags: row.Tags}
		if name != nil {
			params.Name = *name
		}
		if description != nil {
			params.Description = *description
		}
		if tags != nil {
			params.Tags = tags
		}
		if row, err = q.UpdateFlag(ctx, params); err != nil {
			return err
		}
		if err := events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "flag.updated", ProjectID: &p.ID, EntityType: "flag", EntityID: &row.ID,
			Summary: fmt.Sprintf("%s updated flag %s", actor.Name, row.Key),
		}); err != nil {
			return err
		}
		flag, err = s.load(ctx, q, row)
		return err
	})
	return flag, err
}

// SetArchived archives a flag, which takes it out of every SDK snapshot, or
// restores it.
func (s *Service) SetArchived(ctx context.Context, actor identity.User, p projects.Project, key string, archived bool) (Flag, error) {
	var flag Flag
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := s.flag(ctx, q, p, key)
		if err != nil {
			return err
		}
		if row, err = q.SetFlagArchived(ctx, store.SetFlagArchivedParams{ID: row.ID, Archived: archived}); err != nil {
			return err
		}
		if flag, err = s.load(ctx, q, row); err != nil {
			return err
		}
		for _, env := range flag.Environments {
			e, err := q.GetEnvironment(ctx, store.GetEnvironmentParams{ProjectID: p.ID, Key: env.Environment})
			if err != nil {
				return err
			}
			if err := s.changed(ctx, q, p, e.ID, row.Key, int32(env.Version)); err != nil {
				return err
			}
		}
		action, verb := "flag.restored", "restored"
		if archived {
			action, verb = "flag.archived", "archived"
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: action, ProjectID: &p.ID, EntityType: "flag", EntityID: &row.ID,
			Summary: fmt.Sprintf("%s %s flag %s", actor.Name, verb, row.Key),
		})
	})
	if err == nil {
		s.invalidateAll(ctx, flag)
	}
	return flag, err
}

// Config returns a flag's config in one environment.
func (s *Service) Config(ctx context.Context, p projects.Project, key, envKey string) (EnvironmentConfig, error) {
	flag, err := s.Get(ctx, p, key)
	if err != nil {
		return EnvironmentConfig{}, err
	}
	for _, c := range flag.Environments {
		if c.Environment == envKey {
			return c, nil
		}
	}
	return EnvironmentConfig{}, httpx.NotFound("environment %s not found", envKey)
}

// UpdateConfig applies a config to an environment that doesn't require
// approval. It fails with 409 if the config changed since baseVersion.
func (s *Service) UpdateConfig(ctx context.Context, actor identity.User, p projects.Project, key, envKey string, cfg rules.Config, baseVersion int) (EnvironmentConfig, error) {
	env, err := s.projects.Environment(ctx, p.ID, envKey)
	if err != nil {
		return EnvironmentConfig{}, err
	}
	if env.RequiresApproval {
		return EnvironmentConfig{}, httpx.Conflict("%s requires approval; submit a change request instead", env.Name)
	}
	var out EnvironmentConfig
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		flag, err := s.flag(ctx, q, p, key)
		if err != nil {
			return err
		}
		out, err = s.apply(ctx, q, actor, p, flag, env, cfg, baseVersion, "")
		return err
	})
	if err == nil {
		s.cache.Invalidate(ctx, env.ID)
	}
	return out, err
}

// apply checks cfg against the flag, writes it if the config is still at
// baseVersion, and records the change.
func (s *Service) apply(ctx context.Context, q *store.Queries, actor identity.User, p projects.Project, flag store.Flag, env store.Environment, cfg rules.Config, baseVersion int, via string) (EnvironmentConfig, error) {
	if err := s.check(ctx, q, flag, cfg); err != nil {
		return EnvironmentConfig{}, err
	}
	before, err := q.GetFlagConfig(ctx, store.GetFlagConfigParams{FlagID: flag.ID, EnvironmentID: env.ID})
	if err != nil {
		return EnvironmentConfig{}, err
	}
	rulesJSON, err := json.Marshal(nonNil(cfg.Rules))
	if err != nil {
		return EnvironmentConfig{}, err
	}
	row, err := q.UpdateFlagConfig(ctx, store.UpdateFlagConfigParams{
		FlagID: flag.ID, EnvironmentID: env.ID, Enabled: cfg.Enabled, DefaultVariant: cfg.DefaultVariant,
		OffVariant: cfg.OffVariant, Rules: rulesJSON, UpdatedBy: &actor.ID, BaseVersion: int32(baseVersion),
	})
	if postgres.IsNotFound(err) {
		return EnvironmentConfig{}, errStale(before.Version, baseVersion)
	}
	if err != nil {
		return EnvironmentConfig{}, err
	}
	if err := s.changed(ctx, q, p, env.ID, flag.Key, row.Version); err != nil {
		return EnvironmentConfig{}, err
	}
	summary := fmt.Sprintf("%s changed %s in %s", actor.Name, flag.Key, env.Name)
	if before.Enabled != cfg.Enabled {
		summary = fmt.Sprintf("%s turned %s %s in %s", actor.Name, flag.Key, map[bool]string{true: "on", false: "off"}[cfg.Enabled], env.Name)
	}
	if via != "" {
		summary += " " + via
	}
	if err := events.Record(ctx, q, events.Audit{
		Actor: &actor.ID, Action: "flag.config_changed", ProjectID: &p.ID, EntityType: "flag", EntityID: &flag.ID,
		Summary: summary,
		Data:    map[string]any{"environment": env.Key, "version": row.Version, "before": configOf(before), "after": cfg},
	}); err != nil {
		return EnvironmentConfig{}, err
	}
	return toConfig(env.Key, row), nil
}

// check validates a config against the flag's variants.
func (s *Service) check(ctx context.Context, q *store.Queries, flag store.Flag, cfg rules.Config) error {
	variants, err := q.ListVariants(ctx, []uuid.UUID{flag.ID})
	if err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	for _, v := range variants {
		values[v.Key] = v.Value
	}
	return httpx.Invalid(cfg.Check(values))
}

func errStale(current int32, base int) error {
	return httpx.Conflict("the config changed since you loaded it: it's at version %d, not %d; reload and try again", current, base)
}

// changed publishes, through the outbox, that a flag changed in an
// environment.
func (s *Service) changed(ctx context.Context, q *store.Queries, p projects.Project, envID uuid.UUID, flagKey string, version int32) error {
	return events.PublishFlagChanged(ctx, q, events.FlagChanged{
		ProjectID: p.ID, EnvironmentID: envID, FlagKey: flagKey, Version: version, At: time.Now(),
	})
}

func (s *Service) invalidateAll(ctx context.Context, flag Flag) {
	envs, err := s.db.ListFlagConfigs(ctx, []uuid.UUID{flag.ID})
	if err != nil {
		return
	}
	for _, e := range envs {
		s.cache.Invalidate(ctx, e.EnvironmentID)
	}
}

func (s *Service) flag(ctx context.Context, q *store.Queries, p projects.Project, key string) (store.Flag, error) {
	row, err := q.GetFlag(ctx, store.GetFlagParams{ProjectID: p.ID, Key: key})
	if postgres.IsNotFound(err) {
		return row, httpx.NotFound("flag %s not found", key)
	}
	return row, err
}

func (s *Service) load(ctx context.Context, q *store.Queries, row store.Flag) (Flag, error) {
	variants, err := q.ListVariants(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return Flag{}, err
	}
	configs, err := q.ListFlagConfigs(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return Flag{}, err
	}
	flag := Flag{ID: row.ID, Key: row.Key, Name: row.Name, Description: row.Description, Kind: Kind(row.Kind),
		Tags: row.Tags, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ArchivedAt: row.ArchivedAt,
		Variants: make([]Variant, len(variants)), Environments: make([]EnvironmentConfig, len(configs))}
	for i, v := range variants {
		flag.Variants[i] = Variant{Key: v.Key, Value: v.Value, Description: v.Description}
	}
	for i, c := range configs {
		flag.Environments[i] = toConfig(c.EnvironmentKey, store.FlagConfig{
			FlagID: c.FlagID, EnvironmentID: c.EnvironmentID, Enabled: c.Enabled, DefaultVariant: c.DefaultVariant,
			OffVariant: c.OffVariant, Rules: c.Rules, Version: c.Version, UpdatedAt: c.UpdatedAt,
		})
	}
	return flag, nil
}

func toConfig(envKey string, c store.FlagConfig) EnvironmentConfig {
	return EnvironmentConfig{Environment: envKey, Config: configOf(c), Version: int(c.Version), UpdatedAt: c.UpdatedAt}
}

func configOf(c store.FlagConfig) rules.Config {
	cfg := rules.Config{Enabled: c.Enabled, DefaultVariant: c.DefaultVariant, OffVariant: c.OffVariant}
	_ = json.Unmarshal(c.Rules, &cfg.Rules)
	cfg.Rules = nonNil(cfg.Rules)
	return cfg
}

func nonNil(rs []rules.Rule) []rules.Rule {
	if rs == nil {
		return []rules.Rule{}
	}
	return rs
}

// checkValue reports what's wrong with a variant value for the flag's kind.
func checkValue(kind Kind, value json.RawMessage) string {
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return "is required"
	}
	var v any
	if err := json.Unmarshal(value, &v); err != nil {
		return "isn't valid JSON"
	}
	switch kind {
	case "string":
		if _, ok := v.(string); !ok {
			return "must be a JSON string"
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return "must be a number"
		}
	}
	return ""
}
