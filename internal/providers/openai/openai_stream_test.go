package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/models"
)

// sseServer returns a server that streams count SSE data events then [DONE].
func sseServer(t *testing.T, count int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseRecorder is not Flusher")
			return
		}
		for i := 0; i < count; i++ {
			chunk := fmt.Sprintf(`{"id":"c%d","object":"chat.completion.chunk","created":1,"model":"gpt-5","choices":[{"index":0,"delta":{"content":"word%d"},"finish_reason":null}]}`, i, i)
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestChatCompletionStream_ReceivesChunks(t *testing.T) {
	srv := sseServer(t, 3)
	defer srv.Close()

	p := NewProvider("openai", "", srv.URL, DefaultModel)
	req := &models.ChatCompletionRequest{
		Model:    DefaultModel,
		Messages: []models.Message{{Role: "user", Content: "hi"}},
	}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for range ch {
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 chunks, got %d", count)
	}
}

func TestChatCompletionStream_UsesDefaultModel(t *testing.T) {
	srv := sseServer(t, 1)
	defer srv.Close()

	p := NewProvider("openai", "", srv.URL, "my-default")
	// req.Model is empty — should resolve to "my-default"
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	} // drain
	if req.Model != "my-default" {
		t.Errorf("expected model my-default, got %q", req.Model)
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
		// Second call: return a valid SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := NewProvider("openai", "", srv.URL, "bad-model")
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	if err := p.ChatCompletionStream(context.Background(), req, ch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (original + fallback), got %d", callCount)
	}
}

func TestChatCompletionStream_404FallbackDisabled(t *testing.T) {
	srv := newStatusServer(http.StatusNotFound)
	defer srv.Close()

	SetFallbackGetter(func() bool { return false })
	defer SetFallbackGetter(nil)

	p := NewProvider("openai", "", srv.URL, "some-model")
	req := &models.ChatCompletionRequest{Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), req, ch)
	if err == nil {
		t.Error("expected error when fallback disabled on 404")
	}
}

func TestChatCompletionStream_ServerError(t *testing.T) {
	srv := newStatusServer(http.StatusInternalServerError)
	defer srv.Close()

	p := NewProvider("openai", "", srv.URL, "")
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), req, ch)
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestName_OpenAI(t *testing.T) {
	p := NewProvider("my-provider", "", "", "")
	if p.Name() != "my-provider" {
		t.Errorf("expected my-provider, got %q", p.Name())
	}
}
