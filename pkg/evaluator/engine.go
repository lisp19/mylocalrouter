package evaluator

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"localrouter/internal/models"
)

// EvaluateAll executes all configured evaluators concurrently
func EvaluateAll(ctx context.Context, msgs []models.Message, globalTimeoutMs int, evals []Evaluator) map[string]float64 {
	timeout := 100 * time.Millisecond
	if globalTimeoutMs > 0 {
		timeout = time.Duration(globalTimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var g errgroup.Group
	var mu sync.Mutex
	results := make(map[string]float64)

	for _, ev := range evals {
		ev := ev // capture loop variable
		g.Go(func() error {
			res, err := ev.Evaluate(ctx, msgs)
			if err != nil {
				log.Printf("Evaluator %s failed or timed out: %v", ev.Name(), err)
				return nil // do not fail the group, graceful degradation
			}
			mu.Lock()
			results[ev.Name()] = res.Score
			mu.Unlock()
			return nil
		})
	}

	// Wait ignores errors because we swallowed them above to return partial/best-effort
	_ = g.Wait()
	return results
}
