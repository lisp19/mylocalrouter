package router

import (
	"context"
	"testing"

	"localrouter/internal/config"
	"localrouter/internal/models"
	"localrouter/internal/providers"
)

// MockProvider is a simple mock for testing
type MockProvider struct {
	name string
}

func (m *MockProvider) Name() string { return m.name }
func (m *MockProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *MockProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	return nil
}

func TestStrategyEngine_SelectProvider_Fallback(t *testing.T) {
	pMap := map[string]providers.Provider{
		"local_vllm": &MockProvider{name: "local_vllm"},
		"openai":     &MockProvider{name: "openai"},
	}
	engine := NewEngine(pMap)

	// Test 1: No remote config
	req := &models.ChatCompletionRequest{Model: "test-model"}
	p, model, err := engine.SelectProvider(req, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "local_vllm" {
		t.Errorf("expected fallback to local_vllm, got %s", p.Name())
	}
	if model != "test-model" {
		t.Errorf("expected model %s, got %s", "test-model", model)
	}

	// Test 2: Remote config defined strictly "remote"
	rcfg := &config.RemoteStrategy{Strategy: "remote", RemoteModel: "gpt-4"}
	p, model, err = engine.SelectProvider(req, rcfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai provider, got %s", p.Name())
	}
	if model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", model)
	}

	// Test 3: Remote config defined strictly "local"
	rcfg = &config.RemoteStrategy{Strategy: "local", LocalModel: "llama-3"}
	p, model, err = engine.SelectProvider(req, rcfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "local_vllm" {
		t.Errorf("expected local_vllm provider, got %s", p.Name())
	}
	if model != "llama-3" {
		t.Errorf("expected model llama-3, got %s", model)
	}
}

func TestStrategyEngine_SelectProvider_Expression(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{
			Expression: `len(Req.Messages) > 2 ? 'anthropic' : 'google'`,
		},
	}
	// Restore GlobalConfig after test
	defer func() { config.GlobalConfig = nil }()

	pMap := map[string]providers.Provider{
		"anthropic": &MockProvider{name: "anthropic"},
		"google":    &MockProvider{name: "google"},
	}
	engine := NewEngine(pMap)

	rcfg := &config.RemoteStrategy{Strategy: "remote", RemoteModel: "doesn't matter"}

	// Test Expr 1: messages > 2 -> anthropic
	req1 := &models.ChatCompletionRequest{
		Model: "some-model",
		Messages: []models.Message{
			{Role: "user", Content: "1"},
			{Role: "assistant", Content: "2"},
			{Role: "user", Content: "3"},
		},
	}
	p, model, err := engine.SelectProvider(req1, rcfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected expr evaluation to anthropic, got %v", p.Name())
	}
	if model != "some-model" {
		t.Errorf("expected model to remain 'some-model', got %v", model)
	}

	// Test Expr 2: messages <= 2 -> google
	req2 := &models.ChatCompletionRequest{
		Model: "other-model",
		Messages: []models.Message{
			{Role: "user", Content: "1"},
		},
	}
	p, model, err = engine.SelectProvider(req2, rcfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected expr evaluation to google, got %v", p.Name())
	}
	if model != "other-model" {
		t.Errorf("expected model to remain 'other-model', got %v", model)
	}
}
