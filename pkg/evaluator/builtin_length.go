package evaluator

import (
	"context"
	"fmt"
	"strings"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/logger"
)

// BuiltinLengthEvaluator evaluates the length of the latest message
type BuiltinLengthEvaluator struct {
	name      string
	threshold int
}

// NewBuiltinLengthEvaluator creates a new BuiltinLengthEvaluator
func NewBuiltinLengthEvaluator(cfg config.EvaluatorConfig) *BuiltinLengthEvaluator {
	return &BuiltinLengthEvaluator{
		name:      cfg.Name,
		threshold: cfg.Threshold,
	}
}

func (e *BuiltinLengthEvaluator) Name() string {
	return e.name
}

func (e *BuiltinLengthEvaluator) HistoryRounds() int {
	return 0 // Only cares about the current message
}

func (e *BuiltinLengthEvaluator) Evaluate(ctx context.Context, messages []models.Message) (*EvaluationResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Get the last message
	lastMsg := messages[len(messages)-1]

	trimmed := strings.TrimSpace(lastMsg.Content)
	logger.Printf("[Evaluator %s] Verbose Input Message: %s", e.name, trimmed)
	length := len([]rune(trimmed))

	// If length is greater than or equal to threshold, it's considered "complex" (score 1.0)
	// If length is less than threshold, it's considered "simple" (score 0.0)
	score := 0.0
	if length >= e.threshold {
		score = 1.0
	}

	logger.Printf("[Evaluator %s] Verbose Output Score: %f", e.name, score)

	return &EvaluationResult{
		Dimension: e.name,
		Score:     score,
	}, nil
}
