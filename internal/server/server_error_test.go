package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
	"agentic-llm-gateway/internal/router"
)

// --- error-returning stubs ---

type errEngine struct{ err error }

func (e *errEngine) SelectProvider(_ *models.ChatCompletionRequest, _ *config.RemoteStrategy) (providers.Provider, string, error) {
	return nil, "", e.err
}

type errProvider struct{}

func (p *errProvider) Name() string { return "err" }
func (p *errProvider) ChatCompletion(_ context.Context, _ *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	return nil, fmt.Errorf("upstream down")
}
func (p *errProvider) ChatCompletionStream(_ context.Context, _ *models.ChatCompletionRequest, ch chan<- *models.ChatCompletionStreamResponse) error {
	return fmt.Errorf("stream down")
}

var _ router.StrategyEngine = (*errEngine)(nil)

func TestHandleChatCompletions_RoutingError(t *testing.T) {
	srv := NewServer(&stubRM{}, &errEngine{err: fmt.Errorf("routing fail")})
	body, _ := json.Marshal(models.ChatCompletionRequest{
		Model:    "x",
		Messages: []models.Message{{Role: "user", Content: "hi"}},
	})
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleChatCompletions(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleSync_UpstreamError(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	creq := &models.ChatCompletionRequest{Model: "x", Messages: []models.Message{{Role: "user", Content: "hi"}}}
	srv.handleSync(w, req, &errProvider{}, creq)
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestHandleStream_UpstreamError(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	creq := &models.ChatCompletionRequest{Model: "x", Stream: true, Messages: []models.Message{{Role: "user", Content: "hi"}}}
	srv.handleStream(w, req, &errProvider{}, creq)
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}
