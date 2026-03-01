package evaluator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
)

func TestBuiltinLengthEvaluator(t *testing.T) {
	eval := NewBuiltinLengthEvaluator(config.EvaluatorConfig{
		Name:      "test_len",
		Threshold: 10,
	})

	if eval.Name() != "test_len" {
		t.Errorf("expected name test_len")
	}

	tests := []struct {
		name     string
		content  string
		expected float64
	}{
		{"Short string", "hi", 0.0},
		{"Exact threshold", "1234567890", 1.0},
		{"Long string", "this is much longer than 10 chars", 1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := eval.Evaluate(context.Background(), []models.Message{
				{Role: "user", Content: tc.content},
			})
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if res.Score != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, res.Score)
			}
			if res.Dimension != "test_len" {
				t.Errorf("expected dim test_len")
			}
		})
	}
}

func TestLLMAPIEvaluator(t *testing.T) {
	// Mock OpenAI server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			LogitBias map[string]int `json:"logit_bias"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		// Check if template rendered properly (this is simplistic verification)
		if len(req.Messages) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		content := req.Messages[0].Content
		score := "0"
		if len(content) > 30 {
			score = "1"
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": score,
					},
				},
			},
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	eval, err := NewLLMAPIEvaluator(config.EvaluatorConfig{
		Name:           "test_llm",
		Endpoint:       mockServer.URL,
		Model:          "qwen-0.6b",
		TimeoutMs:      1000,
		PromptTemplate: "Context: {{.History}}\nQuery: {{.Current}}",
		HistoryRounds:  1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	msgs := []models.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
		{Role: "user", Content: "Short text"}, // total len of this rendered will be Context: user: Hello\nassistant: Hi!\n\nQuery: Short text => length is > 30 => should score 1
	}

	res, err := eval.Evaluate(context.Background(), msgs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Dimension != "test_llm" {
		t.Errorf("expected test_llm")
	}
	if res.Score != 1 {
		t.Errorf("expected score 1, got %v", res.Score)
	}
}

func TestLLMAPIEvaluatorTimeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // artificially slow
	}))
	defer mockServer.Close()

	eval, _ := NewLLMAPIEvaluator(config.EvaluatorConfig{
		Name:           "timeout_test",
		Endpoint:       mockServer.URL,
		TimeoutMs:      50, // short timeout
		PromptTemplate: "{{.Current}}",
	})

	_, err := eval.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
