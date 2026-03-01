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

// TestLLMAPIEvaluatorOllama tests the default Ollama protocol path
func TestLLMAPIEvaluatorOllama(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Think  bool `json:"think"`
			Stream bool `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Think != false {
			t.Errorf("expected think=false for ollama protocol")
		}
		if req.Stream != false {
			t.Errorf("expected stream=false for ollama protocol")
		}

		if len(req.Messages) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		content := req.Messages[0].Content
		score := "0"
		if len(content) > 30 {
			score = "1"
		}

		// Ollama /api/chat response format
		resp := map[string]interface{}{
			"message": map[string]string{
				"role":    "assistant",
				"content": score,
			},
			"done": true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	eval, err := NewLLMAPIEvaluator(config.EvaluatorConfig{
		Name:           "test_llm_ollama",
		Endpoint:       mockServer.URL,
		Model:          "qwen-0.6b",
		TimeoutMs:      1000,
		PromptTemplate: "Context: {{.History}}\nQuery: {{.Current}}",
		HistoryRounds:  1,
		// Protocol defaults to "ollama"
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	msgs := []models.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
		{Role: "user", Content: "Short text"},
	}

	res, err := eval.Evaluate(context.Background(), msgs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Dimension != "test_llm_ollama" {
		t.Errorf("expected test_llm_ollama")
	}
	if res.Score != 1 {
		t.Errorf("expected score 1, got %v", res.Score)
	}
}

// TestLLMAPIEvaluatorOpenAI tests the OpenAI protocol path
func TestLLMAPIEvaluatorOpenAI(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Messages) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		content := req.Messages[0].Content
		score := "0"
		if len(content) > 30 {
			score = "1"
		}

		// OpenAI response format
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
		Name:           "test_llm_openai",
		Protocol:       "openai",
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
		{Role: "user", Content: "Short text"},
	}

	res, err := eval.Evaluate(context.Background(), msgs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Dimension != "test_llm_openai" {
		t.Errorf("expected test_llm_openai")
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
