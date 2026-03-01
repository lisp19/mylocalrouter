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

func TestLLMLogprobEvaluator(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"logprobs": map[string]interface{}{
						"content": []map[string]interface{}{
							{
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
		Name:           "test_logprob",
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

	// Calculate P(1) = exp(-1.2) / (exp(-0.5) + exp(-1.2)) = 0.301 / (0.606 + 0.301) ≈ 0.33
	expected := 0.3318122278318339
	// math.Exp(-1.2) / (math.Exp(-0.5) + math.Exp(-1.2))

	if res.Score < expected-0.01 || res.Score > expected+0.01 {
		t.Errorf("expected score around %v, got %v", expected, res.Score)
	}
}
