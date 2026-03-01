package strategy

import (
	"testing"

	"agentic-llm-gateway/internal/config"
)

// --- NewResolver ---

func TestNewResolver_DynamicExpression(t *testing.T) {
	cfg := config.ResolutionStrategyConfig{Type: "dynamic_expression"}
	r := NewResolver(cfg)
	if r == nil {
		t.Fatal("expected non-nil resolver for dynamic_expression")
	}
	if r.Name() != "dynamic_expression" {
		t.Errorf("expected name dynamic_expression, got %q", r.Name())
	}
}

func TestNewResolver_StrictLocalFirst(t *testing.T) {
	cfg := config.ResolutionStrategyConfig{Type: "strict_local_first", DefaultProvider: "openai"}
	r := NewResolver(cfg)
	if r == nil {
		t.Fatal("expected non-nil resolver for strict_local_first")
	}
	if r.Name() != "strict_local_first" {
		t.Errorf("expected name strict_local_first, got %q", r.Name())
	}
}

func TestNewResolver_UnknownType(t *testing.T) {
	cfg := config.ResolutionStrategyConfig{Type: "nonexistent"}
	r := NewResolver(cfg)
	if r != nil {
		t.Errorf("expected nil resolver for unknown type, got %v", r)
	}
}

// --- StrictLocalResolver ---

func TestStrictLocalResolver_AllZero_ReturnsLocalVllm(t *testing.T) {
	r := NewStrictLocalResolver(config.ResolutionStrategyConfig{DefaultProvider: "openai"})
	vector := map[string]float64{"complexity": 0.0, "context_rel": 0.0, "length_check": 0.0}
	got := r.Resolve(vector)
	if got != "local_vllm" {
		t.Errorf("expected local_vllm, got %q", got)
	}
}

func TestStrictLocalResolver_NonZero_ReturnsDefault(t *testing.T) {
	r := NewStrictLocalResolver(config.ResolutionStrategyConfig{DefaultProvider: "openai"})
	vector := map[string]float64{"complexity": 1.0, "context_rel": 0.0, "length_check": 0.0}
	got := r.Resolve(vector)
	if got != "openai" {
		t.Errorf("expected openai, got %q", got)
	}
}

func TestStrictLocalResolver_IncompleteVector_ReturnsDefault(t *testing.T) {
	r := NewStrictLocalResolver(config.ResolutionStrategyConfig{DefaultProvider: "google"})
	vector := map[string]float64{"complexity": 0.0} // missing context_rel and length_check
	got := r.Resolve(vector)
	if got != "google" {
		t.Errorf("expected google for incomplete vector, got %q", got)
	}
}

func TestStrictLocalResolver_Name(t *testing.T) {
	r := NewStrictLocalResolver(config.ResolutionStrategyConfig{})
	if r.Name() != "strict_local_first" {
		t.Errorf("unexpected name: %q", r.Name())
	}
}
