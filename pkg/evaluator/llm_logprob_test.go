package evaluator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
)

// TestLLMLogprobEvaluatorOpenAI tests the OpenAI protocol path with logprobs
func TestLLMLogprobEvaluatorOpenAI(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"logprobs": map[string]interface{}{
						"content": []map[string]interface{}{
							{
								"token": "0",
								"top_logprobs": []map[string]interface{}{
									{"token": "0", "logprob": -0.5},
									{"token": "1", "logprob": -1.2},
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	eval, err := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Name:           "test_logprob_openai",
		Protocol:       "openai",
		Endpoint:       mockServer.URL,
		Model:          "test-model",
		PromptTemplate: "{{.Current}}",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	res, err := eval.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Calculate P(1) = exp(-1.2) / (exp(-0.5) + exp(-1.2)) ≈ 0.33
	expected := 0.3318122278318339

	if res.Score < expected-0.01 || res.Score > expected+0.01 {
		t.Errorf("expected score around %v, got %v", expected, res.Score)
	}
}

// TestLLMLogprobEvaluatorOllama tests the Ollama protocol fallback (content-based parsing)
func TestLLMLogprobEvaluatorOllama(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ollama /api/chat response format
		resp := map[string]interface{}{
			"message": map[string]string{
				"role":    "assistant",
				"content": "1",
			},
			"done": true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	eval, err := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Name:           "test_logprob_ollama",
		Endpoint:       mockServer.URL,
		Model:          "test-model",
		PromptTemplate: "{{.Current}}",
		// Protocol defaults to "ollama"
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	res, err := eval.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if res.Score != 1.0 {
		t.Errorf("expected score 1.0, got %v", res.Score)
	}
}
