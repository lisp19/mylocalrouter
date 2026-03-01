package anthropic

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
	got := p.resolveModel("claude-opus")
	if got != "claude-opus" {
		t.Errorf("expected claude-opus, got %q", got)
	}
}

func TestResolveModel_UsesRuntimeDefault(t *testing.T) {
	p := NewProvider("key", "my-model")
	got := p.resolveModel("")
	if got != "my-model" {
		t.Errorf("expected my-model, got %q", got)
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
		go func() { defer wg.Done(); p.SetDefaultModel("new") }()
		go func() { defer wg.Done(); _ = p.resolveModel("") }()
	}
	wg.Wait()
}

// --- Name ---

func TestName(t *testing.T) {
	p := NewProvider("key", "")
	if p.Name() != "anthropic" {
		t.Errorf("expected anthropic, got %q", p.Name())
	}
}

// --- ChatCompletion via mock HTTP server ---

func newAnthropicOKServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			ID:    "resp-1",
			Model: DefaultModel,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "hello"}},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 5, OutputTokens: 3},
		})
	}))
}

func makeSrv(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
}

func TestChatCompletion_UsesDefaultWhenNoModel(t *testing.T) {
	srv := newAnthropicOKServer(t)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "my-default"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "hello" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestChatCompletion_ExplicitModelPassedThrough(t *testing.T) {
	srv := newAnthropicOKServer(t)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}}
	req := &models.ChatCompletionRequest{
		Model:    "claude-opus",
		Messages: []models.Message{{Role: "user", Content: "hi"}},
	}
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
		json.NewEncoder(w).Encode(anthropicResponse{
			ID:    "fallback",
			Model: DefaultModel,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "ok"}},
		})
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "bad-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestChatCompletion_404FallbackDisabled(t *testing.T) {
	srv := makeSrv(http.StatusNotFound)
	defer srv.Close()

	SetFallbackGetter(func() bool { return false })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "some-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Error("expected error when fallback is disabled")
	}
}

func TestChatCompletion_ServerError(t *testing.T) {
	srv := makeSrv(http.StatusInternalServerError)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}}
	req := &models.ChatCompletionRequest{
		Model:    DefaultModel,
		Messages: []models.Message{{Role: "user", Content: "hi"}},
	}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- mapRequest ---

func TestMapRequest_SystemRoleExtracted(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model: DefaultModel,
		Messages: []models.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}
	areq := mapRequest(req)
	if areq.System != "You are helpful." {
		t.Errorf("expected system extracted, got %q", areq.System)
	}
	if len(areq.Messages) != 1 || areq.Messages[0].Role != "user" {
		t.Errorf("expected 1 user message, got %+v", areq.Messages)
	}
}

func TestMapRequest_DefaultMaxTokens(t *testing.T) {
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	areq := mapRequest(req)
	if areq.MaxTokens != 4096 {
		t.Errorf("expected default max_tokens=4096, got %d", areq.MaxTokens)
	}
}

func TestMapRequest_UnknownRoleMappedToUser(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model:    DefaultModel,
		Messages: []models.Message{{Role: "function", Content: "result"}},
	}
	areq := mapRequest(req)
	if len(areq.Messages) != 1 || areq.Messages[0].Role != "user" {
		t.Errorf("expected unknown role mapped to user, got %+v", areq.Messages)
	}
}
