package evaluator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
)

func TestLLMLogprob_Ollama_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ev, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Protocol: "ollama",
		Endpoint: srv.URL,
	})
	_, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Error("expected HTTP error")
	}
}

func TestLLMLogprob_Ollama_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{bad json`))
	}))
	defer srv.Close()

	ev, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Protocol: "ollama",
		Endpoint: srv.URL,
	})
	_, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Error("expected JSON decode error")
	}
}

func TestLLMLogprob_OpenAI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ev, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Protocol: "openai",
		Endpoint: srv.URL,
	})
	_, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Error("expected HTTP error")
	}
}

func TestLLMLogprob_OpenAI_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{bad json`))
	}))
	defer srv.Close()

	ev, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Protocol: "openai",
		Endpoint: srv.URL,
	})
	_, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Error("expected JSON decode error")
	}
}

func TestLLMLogprob_OpenAI_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	ev, _ := NewLLMLogprobEvaluator(config.EvaluatorConfig{
		Protocol: "openai",
		Endpoint: srv.URL,
	})
	_, err := ev.Evaluate(context.Background(), []models.Message{{Role: "user", Content: "A"}})
	if err == nil {
		t.Error("expected empty choices error")
	}
}
