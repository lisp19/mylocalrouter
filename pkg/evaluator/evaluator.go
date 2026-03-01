package evaluator

import (
	"context"

	"agentic-llm-gateway/internal/models"
)

// EvaluationResult stores a single dimension's score
type EvaluationResult struct {
	Dimension string
	Score     float64 // 0.0 ~ 1.0 or binary
}

// Evaluator defines the interface that all intent detection evaluators must implement
type Evaluator interface {
	// Name returns the unique identifier of the evaluator
	Name() string
	// HistoryRounds returns the number of history rounds the evaluator requires
	HistoryRounds() int
	// Evaluate executes the evaluation logic. messages in the context.
	Evaluate(ctx context.Context, messages []models.Message) (*EvaluationResult, error)
}
