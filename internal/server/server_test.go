package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
	"agentic-llm-gateway/internal/router"
)

// --- minimal stubs ---

type stubRM struct{}

func (s *stubRM) GetStrategy() *config.RemoteStrategy {
	return &config.RemoteStrategy{Strategy: "remote", RemoteProvider: "mock"}
}

type stubProvider struct{}

func (p *stubProvider) Name() string { return "mock" }
func (p *stubProvider) ChatCompletion(_ context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	return &models.ChatCompletionResponse{
		ID:    "cmpl-stub",
		Model: req.Model,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{{Message: models.Message{Role: "assistant", Content: "ok"}}},
	}, nil
}
func (p *stubProvider) ChatCompletionStream(_ context.Context, _ *models.ChatCompletionRequest, ch chan<- *models.ChatCompletionStreamResponse) error {
	go func() { close(ch) }()
	return nil
}

type stubEngine struct{}

func (e *stubEngine) SelectProvider(_ *models.ChatCompletionRequest, _ *config.RemoteStrategy) (providers.Provider, string, error) {
	return &stubProvider{}, "stub-model", nil
}

// verify interface satisfaction at compile time
var _ StrategyManager = (*stubRM)(nil)
var _ router.StrategyEngine = (*stubEngine)(nil)

// --- tests ---

func newTestServer() *Server {
	return NewServer(&stubRM{}, &stubEngine{})
}

func chatReqBody(t *testing.T, stream bool) *bytes.Reader {
	t.Helper()
	body, _ := json.Marshal(models.ChatCompletionRequest{
		Model:    "stub-model",
		Stream:   stream,
		Messages: []models.Message{{Role: "user", Content: "hello"}},
	})
	return bytes.NewReader(body)
}

func TestHandleChatCompletions_Sync(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/v1/chat/completions", chatReqBody(t, false))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp models.ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ID != "cmpl-stub" {
		t.Errorf("unexpected id: %q", resp.ID)
	}
}

func TestHandleChatCompletions_InvalidJSON(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte("!bad")))
	w := httptest.NewRecorder()

	srv.handleChatCompletions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChatCompletions_Stream(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/v1/chat/completions", chatReqBody(t, true))
	w := httptest.NewRecorder()

	srv.handleChatCompletions(w, req)

	// httptest.ResponseRecorder implements Flusher, so streaming should succeed.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
