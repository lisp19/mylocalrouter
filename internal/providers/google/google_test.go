package google

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

func TestResolveModel_NonEmpty(t *testing.T) {
	p := NewProvider("key", "")
	got := p.resolveModel("gemini-pro")
	if got != "gemini-pro" {
		t.Errorf("expected gemini-pro, got %q", got)
	}
}

func TestResolveModel_UsesRuntimeDefault(t *testing.T) {
	p := NewProvider("key", "my-runtime-model")
	got := p.resolveModel("")
	if got != "my-runtime-model" {
		t.Errorf("expected my-runtime-model, got %q", got)
	}
}

func TestResolveModel_FallsBackToConst(t *testing.T) {
	p := NewProvider("key", "")
	got := p.resolveModel("")
	if got != DefaultModel {
		t.Errorf("expected %q, got %q", DefaultModel, got)
	}
}

// --- SetDefaultModel thread safety ---

func TestSetDefaultModel_Race(t *testing.T) {
	p := NewProvider("key", "init")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); p.SetDefaultModel("updated") }()
		go func() { defer wg.Done(); _ = p.resolveModel("") }()
	}
	wg.Wait()
}

// --- Name ---

func TestName(t *testing.T) {
	p := NewProvider("key", "")
	if p.Name() != "google" {
		t.Errorf("expected google, got %q", p.Name())
	}
}

// --- ChatCompletion via mock HTTP server ---

func newGeminiOKServer(t *testing.T, model string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := geminiResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{Parts: []struct {
						Text string `json:"text"`
					}{{Text: "hello"}}},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func makeSrv(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
}

func TestChatCompletion_UsesDefaultWhenNoModel(t *testing.T) {
	srv := newGeminiOKServer(t, DefaultModel)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "my-default"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected choices")
	}
}

func TestChatCompletion_ExplicitModelPassedThrough(t *testing.T) {
	srv := newGeminiOKServer(t, "gemini-pro")
	defer srv.Close()

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "ignored"}
	req := &models.ChatCompletionRequest{Model: "gemini-pro", Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatCompletion_404FallbackEnabled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{})
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "bad-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestChatCompletion_404FallbackDisabled(t *testing.T) {
	srv := makeSrv(http.StatusNotFound)
	defer srv.Close()

	SetFallbackGetter(func() bool { return false })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "some-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Error("expected error when fallback disabled")
	}
}

func TestChatCompletion_ServerError(t *testing.T) {
	srv := makeSrv(http.StatusInternalServerError)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}}
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- redactKey ---

func TestRedactKey(t *testing.T) {
	p := &Provider{apiKey: "secret123"}
	got := p.redactKey("error: secret123 not found")
	if got != "error: *** not found" {
		t.Errorf("expected key to be redacted, got %q", got)
	}
}

func TestRedactKey_EmptyKey(t *testing.T) {
	p := &Provider{apiKey: ""}
	s := "no key here"
	if got := p.redactKey(s); got != s {
		t.Errorf("expected unchanged string, got %q", got)
	}
}
