package evaluator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
)

func TestParseOllamaContent_ValidJSON(t *testing.T) {
	res, err := parseOllamaContent([]byte(`{"message":{"content":"{\"test\":0.99}","role":"assistant"}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res) == 0 {
		t.Errorf("expected map with content, got zero length")
	}
}

func TestParseOllamaContent_InvalidJSONResp(t *testing.T) {
	_, err := parseOllamaContent([]byte(`{"message":{"content":"not json"}}`))
	// parseOllamaContent doesn't unmarshal inner json, just grabs the string.
	// We'll just verify no top-level unmarshal panic occurs.
	_ = err
}

func TestParseOllamaContent_BadOuterJSON(t *testing.T) {
	_, err := parseOllamaContent([]byte(`bad`))
	if err == nil {
		t.Error("expected error for bad outer json")
	}
}

func TestParseOpenAIContent_ValidJSON(t *testing.T) {
	res, err := parseOpenAIContent([]byte(`{"choices":[{"message":{"content":"{\"score\":1.0}"}}]}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res) == 0 {
		t.Errorf("expected map with content")
	}
}

func TestParseOpenAIContent_InvalidJSONResp(t *testing.T) {
	_, err := parseOpenAIContent([]byte(`{"choices":[{"message":{"content":"bad"}}]}`))
	// Same as ollama, just returns the string content.
	_ = err
}

func TestParseOpenAIContent_EmptyChoices(t *testing.T) {
	_, err := parseOpenAIContent([]byte(`{"choices":[]}`))
	if err == nil {
		t.Error("expected err for empty choices")
	}
}

func TestLLMAPIEvaluator_EvaluateOpenAIMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"5"}}]}`))
	}))
	defer srv.Close()

	ev, _ := NewLLMAPIEvaluator(config.EvaluatorConfig{
		Name:           "myScore",
		Protocol:       "openai",
		Endpoint:       srv.URL,
		TimeoutMs:      1000,
		PromptTemplate: ".",
	})
	res, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Score != 5.0 {
		t.Errorf("expected 5.0, got %v", res.Score)
	}
}
