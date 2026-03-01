package google

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/models"
)

// sseGeminiServer streams Gemini-style SSE events then closes.
func sseGeminiServer(t *testing.T, texts []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, text := range texts {
			payload := fmt.Sprintf(
				`{"candidates":[{"content":{"parts":[{"text":"%s"}]},"finishReason":""}]}`,
				text,
			)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		// Emit an empty event and [DONE] to check termination
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

func TestChatCompletionStream_ReceivesChunks(t *testing.T) {
	srv := sseGeminiServer(t, []string{"hello", " world", "!"})
	defer srv.Close()

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: DefaultModel}
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
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
	srv := sseGeminiServer(t, nil)
	defer srv.Close()

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "my-default"}
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
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "bad-model"}
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

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}, defaultModel: "some-model"}
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

	p := &Provider{baseURL: srv.URL + "/", client: &http.Client{}}
	req := &models.ChatCompletionRequest{Model: DefaultModel, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), req, ch)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// TestMapRequest_RoleMappings verifies Gemini role mapping.
func TestMapRequest_RoleMappings(t *testing.T) {
	cases := []struct {
		inRole  string
		outRole string
	}{
		{"assistant", "model"},
		{"system", "user"},
		{"function", "user"},
		{"user", "user"},
	}
	for _, tc := range cases {
		req := &models.ChatCompletionRequest{
			Model:    DefaultModel,
			Messages: []models.Message{{Role: tc.inRole, Content: "x"}},
		}
		greq := mapRequest(req)
		if len(greq.Contents) == 0 {
			t.Fatalf("expected content for role %q", tc.inRole)
		}
		if greq.Contents[0].Role != tc.outRole {
			t.Errorf("role %q: expected %q, got %q", tc.inRole, tc.outRole, greq.Contents[0].Role)
		}
	}
}
