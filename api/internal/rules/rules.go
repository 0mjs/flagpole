// Package rules defines how a flag decides what to serve: the targeting
// rules stored with each environment's config, how they're checked, and how
// they're evaluated for a context. It has no I/O, so the API and the SDKs'
// snapshot share one definition.
package rules

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// Operator compares a context attribute with a condition's values.
type Operator string

const (
	In         Operator = "in"
	NotIn      Operator = "not_in"
	Contains   Operator = "contains"
	StartsWith Operator = "starts_with"
	EndsWith   Operator = "ends_with"
	GT         Operator = "gt"
	GTE        Operator = "gte"
	LT         Operator = "lt"
	LTE        Operator = "lte"
)

func (Operator) Enum() []any {
	return []any{In, NotIn, Contains, StartsWith, EndsWith, GT, GTE, LT, LTE}
}

func (o Operator) numeric() bool { return o == GT || o == GTE || o == LT || o == LTE }

// Condition matches when the context's attribute satisfies the operator for
// any of the values. "key" is the context's key.
type Condition struct {
	Attribute string   `json:"attribute" validate:"required,max=100" example:"country"`
	Operator  Operator `json:"operator" validate:"required"`
	Values    []string `json:"values" validate:"required,min=1,max=100"`
}

// Weight is a share of a percentage rollout.
type Weight struct {
	Variant string `json:"variant" validate:"required"`
	Weight  int    `json:"weight" validate:"gte=0,lte=100" doc:"A percentage; a rollout's weights add up to 100."`
}

// Rule serves a variant, or splits a rollout, to contexts that match all
// its conditions. A rule with no conditions matches everyone.
type Rule struct {
	Description string      `json:"description,omitempty" validate:"max=200"`
	Conditions  []Condition `json:"conditions" validate:"max=20"`
	Variant     string      `json:"variant,omitempty" doc:"Serve this variant. Set this or rollout."`
	Rollout     []Weight    `json:"rollout,omitempty" validate:"max=20" doc:"Split matching contexts between variants by weight. Set this or variant."`
}

// Config is how a flag behaves in one environment.
type Config struct {
	Enabled        bool   `json:"enabled"`
	DefaultVariant string `json:"default_variant" validate:"required" doc:"Served when the flag is on and no rule matches."`
	OffVariant     string `json:"off_variant" validate:"required" doc:"Served when the flag is off."`
	Rules          []Rule `json:"rules" validate:"max=50"`
}

// Check reports what's wrong with c for a flag with these variants, by field.
func (c Config) Check(variants map[string]json.RawMessage) map[string]string {
	problems := map[string]string{}
	known := func(field, key string) {
		if _, ok := variants[key]; !ok {
			problems[field] = fmt.Sprintf("is %q, which isn't a variant of this flag", key)
		}
	}
	known("default_variant", c.DefaultVariant)
	known("off_variant", c.OffVariant)
	for i, r := range c.Rules {
		at := fmt.Sprintf("rules[%d]", i)
		switch {
		case r.Variant != "" && len(r.Rollout) > 0:
			problems[at] = "sets both variant and rollout; set one"
		case r.Variant == "" && len(r.Rollout) == 0:
			problems[at] = "needs a variant or a rollout"
		case r.Variant != "":
			known(at+".variant", r.Variant)
		default:
			total := 0
			for j, w := range r.Rollout {
				known(fmt.Sprintf("%s.rollout[%d].variant", at, j), w.Variant)
				total += w.Weight
			}
			if total != 100 {
				problems[at+".rollout"] = fmt.Sprintf("weights add up to %d; they must add up to 100", total)
			}
		}
		for j, cond := range r.Conditions {
			if !cond.Operator.numeric() {
				continue
			}
			for _, v := range cond.Values {
				if _, err := strconv.ParseFloat(v, 64); err != nil {
					problems[fmt.Sprintf("%s.conditions[%d].values", at, j)] = fmt.Sprintf("%q isn't a number, and %s compares numbers", v, cond.Operator)
					break
				}
			}
		}
	}
	return problems
}

// Context is who a flag is evaluated for.
type Context struct {
	Key        string         `json:"key" validate:"required,max=200" doc:"Identifies the user or thing; percentage rollouts give the same key the same variant." example:"user-42"`
	Attributes map[string]any `json:"attributes,omitempty" doc:"Anything rules can target, such as country or plan."`
}

// Reason says why a variant was served.
type Reason string

const (
	ReasonOff      Reason = "off"
	ReasonRule     Reason = "rule"
	ReasonRollout  Reason = "rollout"
	ReasonDefault  Reason = "default"
	ReasonNotFound Reason = "not_found"
)

func (Reason) Enum() []any {
	return []any{ReasonOff, ReasonRule, ReasonRollout, ReasonDefault, ReasonNotFound}
}

// Result is what a flag served, and why.
type Result struct {
	Variant string          `json:"variant"`
	Value   json.RawMessage `json:"value"`
	Reason  Reason          `json:"reason"`
	Rule    *int            `json:"rule,omitempty" doc:"The index of the rule that matched."`
}

// Evaluate decides what a flag serves to ctx. flagKey salts the rollout
// hash, so the same user lands in different buckets for different flags.
func Evaluate(flagKey string, c Config, variants map[string]json.RawMessage, ctx Context) Result {
	serve := func(variant string, reason Reason, rule *int) Result {
		return Result{Variant: variant, Value: variants[variant], Reason: reason, Rule: rule}
	}
	if !c.Enabled {
		return serve(c.OffVariant, ReasonOff, nil)
	}
	for i, r := range c.Rules {
		if !matches(r.Conditions, ctx) {
			continue
		}
		i := i
		if r.Variant != "" {
			return serve(r.Variant, ReasonRule, &i)
		}
		bucket := int(xxhash.Sum64String(flagKey+"/"+ctx.Key) % 100)
		for _, w := range r.Rollout {
			if bucket < w.Weight {
				return serve(w.Variant, ReasonRollout, &i)
			}
			bucket -= w.Weight
		}
	}
	return serve(c.DefaultVariant, ReasonDefault, nil)
}

func matches(conds []Condition, ctx Context) bool {
	for _, cond := range conds {
		if !conditionMatches(cond, ctx) {
			return false
		}
	}
	return true
}

func conditionMatches(cond Condition, ctx Context) bool {
	var actual string
	if cond.Attribute == "key" {
		actual = ctx.Key
	} else {
		v, ok := ctx.Attributes[cond.Attribute]
		if !ok || v == nil {
			return cond.Operator == NotIn
		}
		actual = fmt.Sprint(v)
	}
	if cond.Operator == NotIn {
		for _, want := range cond.Values {
			if actual == want {
				return false
			}
		}
		return true
	}
	for _, want := range cond.Values {
		if compare(cond.Operator, actual, want) {
			return true
		}
	}
	return false
}

func compare(op Operator, actual, want string) bool {
	switch op {
	case In:
		return actual == want
	case Contains:
		return strings.Contains(actual, want)
	case StartsWith:
		return strings.HasPrefix(actual, want)
	case EndsWith:
		return strings.HasSuffix(actual, want)
	}
	a, errA := strconv.ParseFloat(actual, 64)
	w, errW := strconv.ParseFloat(want, 64)
	if errA != nil || errW != nil {
		return false
	}
	switch op {
	case GT:
		return a > w
	case GTE:
		return a >= w
	case LT:
		return a < w
	case LTE:
		return a <= w
	}
	return false
}
