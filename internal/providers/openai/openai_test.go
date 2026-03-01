package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"agentic-llm-gateway/internal/models"
)

// --- resolveModel ---

func TestResolveModel_ReqModelNonEmpty(t *testing.T) {
	p := NewProvider("openai", "key", "", "")
	got := p.resolveModel("gpt-4o-mini")
	if got != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini, got %q", got)
	}
}

func TestResolveModel_UsesRuntimeDefault(t *testing.T) {
	p := NewProvider("openai", "key", "", "my-runtime-model")
	got := p.resolveModel("")
	if got != "my-runtime-model" {
		t.Errorf("expected my-runtime-model, got %q", got)
	}
}

func TestResolveModel_FallsBackToConst(t *testing.T) {
	p := NewProvider("openai", "key", "", "")
	got := p.resolveModel("")
	if got != DefaultModel {
		t.Errorf("expected DefaultModel %q, got %q", DefaultModel, got)
	}
}

// --- SetDefaultModel thread safety ---

func TestSetDefaultModel_Race(t *testing.T) {
	p := NewProvider("openai", "key", "", "initial")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.SetDefaultModel("updated")
		}()
		go func() {
			defer wg.Done()
			_ = p.resolveModel("") // concurrent read
		}()
	}
	wg.Wait()
}

// --- ChatCompletion with mock HTTP server ---

func newOKServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.ChatCompletionResponse{
			ID:    "cmpl-test",
			Model: req.Model,
		})
	}))
}

func newStatusServer(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
}

func TestChatCompletion_UsesDefaultWhenNoModel(t *testing.T) {
	srv := newOKServer(t)
	defer srv.Close()

	p := NewProvider("openai", "", srv.URL, "my-default")
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "my-default" {
		t.Errorf("expected model my-default, got %q", resp.Model)
	}
}

func TestChatCompletion_ExplicitModelPassedThrough(t *testing.T) {
	srv := newOKServer(t)
	defer srv.Close()

	p := NewProvider("openai", "", srv.URL, "ignored-default")
	req := &models.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []models.Message{{Role: "user", Content: "hi"}},
	}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Errorf("expected model gpt-4o-mini, got %q", resp.Model)
	}
}

func TestChatCompletion_404FallbackEnabled(t *testing.T) {
	// First call → 404; second call (fallback) → 200
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req models.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.ChatCompletionResponse{ID: "fallback", Model: req.Model})
	}))
	defer srv.Close()

	// Ensure fallback is enabled.
	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := NewProvider("openai", "", srv.URL, "bad-model")
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if resp.Model != DefaultModel {
		t.Errorf("expected fallback model %q, got %q", DefaultModel, resp.Model)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (original + retry), got %d", callCount)
	}
}

func TestChatCompletion_404FallbackDisabled(t *testing.T) {
	srv := newStatusServer(http.StatusNotFound)
	defer srv.Close()

	SetFallbackGetter(func() bool { return false })
	defer SetFallbackGetter(nil)

	p := NewProvider("openai", "", srv.URL, "some-model")
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Error("expected error when 404 fallback is disabled, got nil")
	}
}
