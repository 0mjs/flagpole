package rules

import (
	"encoding/json"
	"fmt"
	"testing"
)

var boolVariants = map[string]json.RawMessage{"on": json.RawMessage("true"), "off": json.RawMessage("false")}

func TestEvaluate(t *testing.T) {
	cfg := Config{
		Enabled: true, DefaultVariant: "off", OffVariant: "off",
		Rules: []Rule{
			{Conditions: []Condition{{Attribute: "key", Operator: In, Values: []string{"vip-1"}}}, Variant: "on"},
			{Conditions: []Condition{{Attribute: "plan", Operator: In, Values: []string{"pro", "team"}}, {Attribute: "age", Operator: GTE, Values: []string{"18"}}}, Variant: "on"},
			{Conditions: []Condition{{Attribute: "email", Operator: EndsWith, Values: []string{"@example.com"}}}, Rollout: []Weight{{"on", 50}, {"off", 50}}},
		},
	}
	for _, tt := range []struct {
		name   string
		cfg    Config
		ctx    Context
		want   string
		reason Reason
	}{
		{"off", Config{OffVariant: "off", DefaultVariant: "on"}, Context{Key: "x"}, "off", ReasonOff},
		{"key rule", cfg, Context{Key: "vip-1"}, "on", ReasonRule},
		{"all conditions", cfg, Context{Key: "a", Attributes: map[string]any{"plan": "pro", "age": 30}}, "on", ReasonRule},
		{"one condition fails", cfg, Context{Key: "a", Attributes: map[string]any{"plan": "pro", "age": 12}}, "off", ReasonDefault},
		{"missing attribute", cfg, Context{Key: "a"}, "off", ReasonDefault},
	} {
		got := Evaluate("flag", tt.cfg, boolVariants, tt.ctx)
		if got.Variant != tt.want || got.Reason != tt.reason {
			t.Errorf("%s: got %s/%s, want %s/%s", tt.name, got.Variant, got.Reason, tt.want, tt.reason)
		}
	}
}

// A rollout is stable per key and close to its weights across many keys.
func TestRolloutIsStableAndBalanced(t *testing.T) {
	cfg := Config{Enabled: true, DefaultVariant: "off", OffVariant: "off",
		Rules: []Rule{{Rollout: []Weight{{"on", 30}, {"off", 70}}}}}
	on := 0
	for i := range 10000 {
		ctx := Context{Key: fmt.Sprintf("user-%d", i)}
		first := Evaluate("checkout", cfg, boolVariants, ctx)
		if again := Evaluate("checkout", cfg, boolVariants, ctx); again.Variant != first.Variant {
			t.Fatalf("%s changed variant between calls", ctx.Key)
		}
		if first.Variant == "on" {
			on++
		}
	}
	if on < 2800 || on > 3200 {
		t.Fatalf("30%% rollout served on to %d of 10000", on)
	}
}

func TestCheck(t *testing.T) {
	cfg := Config{
		DefaultVariant: "on", OffVariant: "maybe",
		Rules: []Rule{
			{Variant: "on", Rollout: []Weight{{"on", 100}}},
			{},
			{Rollout: []Weight{{"on", 60}, {"off", 30}}},
			{Conditions: []Condition{{Attribute: "age", Operator: GT, Values: []string{"old"}}}, Variant: "on"},
		},
	}
	got := cfg.Check(boolVariants)
	for field, want := range map[string]string{
		"off_variant":                   `is "maybe", which isn't a variant of this flag`,
		"rules[0]":                      "sets both variant and rollout; set one",
		"rules[1]":                      "needs a variant or a rollout",
		"rules[2].rollout":              "weights add up to 90; they must add up to 100",
		"rules[3].conditions[0].values": `"old" isn't a number, and gt compares numbers`,
	} {
		if got[field] != want {
			t.Errorf("%s: got %q, want %q", field, got[field], want)
		}
	}
	if len(got) != 5 {
		t.Errorf("problems: %v", got)
	}
}
