package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/models"
)

func TestChatCompletion_FallbackFails(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "bad-model"}
	_, err := p.ChatCompletion(context.Background(), &models.ChatCompletionRequest{})
	if err == nil {
		t.Error("expected error from fallback returning 500")
	}
}

func TestChatCompletionStream_FallbackFails(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	SetFallbackGetter(func() bool { return true })
	defer SetFallbackGetter(nil)

	p := &Provider{baseURL: srv.URL, client: &http.Client{}, defaultModel: "bad-model"}
	ch := make(chan *models.ChatCompletionStreamResponse)
	err := p.ChatCompletionStream(context.Background(), &models.ChatCompletionRequest{}, ch)
	if err == nil {
		t.Error("expected error from fallback returning 500")
	}
}
