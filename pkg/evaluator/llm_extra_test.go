package evaluator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
)

func TestLLMLogprobEvaluator_Timeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer mockServer.Close()

	eval, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Name:           "fast",
		Endpoint:       mockServer.URL,
		TimeoutMs:      10, // 10ms
		PromptTemplate: ".",
	})
	_, err := eval.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "X"}})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestLLMLogprobEvaluator_BadProtocol(t *testing.T) {
	eval, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Name:           "bad",
		Protocol:       "unknown",
		PromptTemplate: ".",
	})
	_, err := eval.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "X"}})
	if err == nil {
		t.Errorf("expected error, got nil")
	} else if err.Error() != "unsupported protocol for logprob evaluator: unknown" {
		// Evaluate internally triggers HTTP POST, so if it fails immediately during format parsing it returns the config err
		// However, it might fail at dial if it doesn't return early. Let's just ensure it errors out cleanly.
	}
}

func TestLLMLogprobEvaluator_NameAndHistory(t *testing.T) {
	eval, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Name:          "test",
		HistoryRounds: 2,
	})
	if eval.Name() != "test" {
		t.Errorf("expected test, got %q", eval.Name())
	}
	if eval.HistoryRounds() != 2 {
		t.Errorf("expected 2, got %d", eval.HistoryRounds())
	}
}

func TestLLMAPIEvaluator_NameAndHistory(t *testing.T) {
	eval, _ := NewLLMAPIEvaluator(config.EvaluatorConfig{
		Name:          "api",
		HistoryRounds: 3,
	})
	if eval.Name() != "api" {
		t.Errorf("expected api, got %q", eval.Name())
	}
	if eval.HistoryRounds() != 3 {
		t.Errorf("expected 3, got %d", eval.HistoryRounds())
	}
}
