package strategy

import (
	"testing"

	"localrouter/internal/config"
)

func TestExpressionResolver(t *testing.T) {
	cfg := config.ResolutionStrategyConfig{
		Type: "dynamic_expression",
		Rules: []config.ResolutionRuleConfig{
			{Condition: "complexity == 0 && length_check < 50", TargetProvider: "local"},
			{Condition: "complexity == 1 || context_rel == 1", TargetProvider: "claude"},
		},
		DefaultProvider: "openai",
	}

	resolver := NewExpressionResolver(cfg)

	tests := []struct {
		name     string
		vector   map[string]float64
		expected string
	}{
		{
			"Simple local rule match",
			map[string]float64{"complexity": 0, "length_check": 20},
			"local",
		},
		{
			"Complex rule match",
			map[string]float64{"complexity": 1, "length_check": 10},
			"claude",
		},
		{
			"Context rule match",
			map[string]float64{"complexity": 0, "length_check": 100, "context_rel": 1},
			"claude",
		},
		{
			"No rules match (fallback)",
			map[string]float64{"complexity": 0, "length_check": 60, "context_rel": 0},
			"openai",
		},
		{
			"Missing dimension (fail-safe)",
			map[string]float64{"context_rel": 1}, // missing complexity
			"claude",                             // fails local match but context_rel == 1 is true so matches claude
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := resolver.Resolve(tc.vector)
			if res != tc.expected {
				t.Errorf("Expected %s, got %s for vector %v", tc.expected, res, tc.vector)
			}
		})
	}
}
