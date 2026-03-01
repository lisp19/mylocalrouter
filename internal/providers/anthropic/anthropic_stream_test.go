package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/models"
)

// sseAnthropicServer returns a server that emits Anthropic-style SSE events.
func sseAnthropicServer(t *testing.T, texts []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, text := range texts {
			event := fmt.Sprintf(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"%s"}}`, text)
			fmt.Fprintf(w, "data: %s\n\n", event)
			if flusher != nil {
				flusher.Flush()
			}
		}
		// Send a non-content event to verify it's skipped
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

func TestChatCompletionStream_ReceivesChunks(t *testing.T) {
	srv := sseAnthropicServer(t, []string{"hello", " world"})
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: DefaultModel}
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for chunk := range ch {
		if len(chunk.Choices) == 0 {
			t.Error("expected choices in chunk")
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 text delta chunks, got %d", count)
	}
}

func TestChatCompletionStream_UsesDefaultModel(t *testing.T) {
	srv := sseAnthropicServer(t, nil)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "my-default"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	if req.Model != "my-default" {
		t.Errorf("expected my-default, got %q", req.Model)
	}
}

func TestChatCompletionStream_404FallbackEnabled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// empty stream
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "bad-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	for range ch {
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestChatCompletionStream_404FallbackDisabled(t *testing.T) {
	srv := makeSrv(http.StatusNotFound)
	defer srv.Close()

	SetFallbackGetter(func() bool { return false })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "some-model"}
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), req, ch)
	if err == nil {
		t.Error("expected error when fallback disabled")
	}
}

func TestChatCompletionStream_ServerError(t *testing.T) {
	srv := makeSrv(http.StatusInternalServerError)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL, client: &http.Client{}}
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), req, ch)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestName_Anthropic(t *testing.T) {
	p := NewProvider("key", "")
	if p.Name() != "anthropic" {
		t.Errorf("expected anthropic, got %q", p.Name())
	}
}
