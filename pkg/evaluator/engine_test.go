package evaluator

import (
	"context"
	"testing"
	"time"

	"agentic-llm-gateway/internal/models"
)

// --- stubEvaluator is a minimal in-process evaluator for EvaluateAll tests ---

type stubEvaluator struct {
	name      string
	score     float64
	sleepMs   int
	returnErr bool
}

func (s *stubEvaluator) Name() string       { return s.name }
func (s *stubEvaluator) HistoryRounds() int { return 0 }
func (s *stubEvaluator) Evaluate(_ context.Context, _ []models.Message) (*EvaluationResult, error) {
	if s.sleepMs > 0 {
		time.Sleep(time.Duration(s.sleepMs) * time.Millisecond)
	}
	if s.returnErr {
		return nil, context.DeadlineExceeded
	}
	return &EvaluationResult{Score: s.score}, nil
}

func TestEvaluateAll_SingleEvaluator(t *testing.T) {
	evals := []Evaluator{&stubEvaluator{name: "test", score: 0.8}}
	results := EvaluateAll(context.Background(), nil, 1000, evals)
	if results["test"] != 0.8 {
		t.Errorf("expected 0.8, got %v", results["test"])
	}
}

func TestEvaluateAll_MultipleEvaluators(t *testing.T) {
	evals := []Evaluator{
		&stubEvaluator{name: "a", score: 1.0},
		&stubEvaluator{name: "b", score: 0.5},
	}
	results := EvaluateAll(context.Background(), nil, 1000, evals)
	if results["a"] != 1.0 || results["b"] != 0.5 {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestEvaluateAll_FailingEvaluatorIsTolerated(t *testing.T) {
	evals := []Evaluator{
		&stubEvaluator{name: "good", score: 0.7},
		&stubEvaluator{name: "bad", returnErr: true},
	}
	results := EvaluateAll(context.Background(), nil, 1000, evals)
	if results["good"] != 0.7 {
		t.Errorf("expected good=0.7, got %v", results["good"])
	}
	if _, ok := results["bad"]; ok {
		t.Error("bad evaluator result should be absent")
	}
}

func TestEvaluateAll_EmptyEvaluators(t *testing.T) {
	results := EvaluateAll(context.Background(), nil, 1000, nil)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestEvaluateAll_GlobalTimeout(t *testing.T) {
	// Evaluator sleeps longer than the timeout; should gracefully degrade without panic.
	evals := []Evaluator{
		&stubEvaluator{name: "slow", sleepMs: 200},
	}
	_ = EvaluateAll(context.Background(), nil, 50 /* ms */, evals)
}

func TestEvaluateAll_ZeroTimeout_UsesDefault(t *testing.T) {
	evals := []Evaluator{&stubEvaluator{name: "x", score: 1.0}}
	// globalTimeoutMs=0 uses the internal 10s default — should not panic
	results := EvaluateAll(context.Background(), nil, 0, evals)
	if results["x"] != 1.0 {
		t.Errorf("expected 1.0, got %v", results["x"])
	}
}
