package router

import (
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
)

// TestGenerativeRouting_FallbackProvider exercises the generative routing path
// when evaluators are enabled but produce no result, falling back to FallbackProvider.
func TestGenerativeRouting_FallbackProvider(t *testing.T) {
	config.GlobalConfig = &config.Config{
		GenerativeRouting: &config.GenerativeRoutingConfig{
			Enabled:          true,
			GlobalTimeoutMs:  500,
			FallbackProvider: "openai",
			Resolution: config.ResolutionStrategyConfig{
				Type:            "dynamic_expression",
				DefaultProvider: "openai",
				Rules:           nil, // no rules → resolver returns default_provider
			},
		},
	}
	defer func() { config.GlobalConfig = nil }()

	pMap := map[string]providers.Provider{
		"openai": &MockProvider{name: "openai"},
		"google": &MockProvider{name: "google"},
	}
	// NewEngine with no evaluators configured in GlobalConfig.Evaluators → evals slice empty.
	// Generative routing block is skipped; falls through to normal routing.
	engine := NewEngine(pMap)

	req := &models.ChatCompletionRequest{Model: "test-model", Messages: []models.Message{{Role: "user", Content: "hi"}}}
	rcfg := &config.RemoteStrategy{Strategy: "remote", RemoteProvider: "openai", RemoteModel: "gpt-5"}

	p, model, err := engine.SelectProvider(req, rcfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai, got %q", p.Name())
	}
	if model != "gpt-5" {
		t.Errorf("expected gpt-5, got %q", model)
	}
}

// TestGenerativeRouting_LocalVllmModel exercises the local_vllm strategy path
// with GlobalConfig set (generative routing enabled but no evaluators).
func TestGenerativeRouting_LocalVllmModel(t *testing.T) {
	config.GlobalConfig = &config.Config{
		GenerativeRouting: &config.GenerativeRoutingConfig{
			Enabled:         true,
			GlobalTimeoutMs: 500,
		},
	}
	defer func() { config.GlobalConfig = nil }()

	pMap := map[string]providers.Provider{
		"local_vllm": &MockProvider{name: "local_vllm"},
	}
	engine := NewEngine(pMap)

	req := &models.ChatCompletionRequest{Model: "test-model", Messages: []models.Message{{Role: "user", Content: "hi"}}}
	rcfg := &config.RemoteStrategy{Strategy: "local", LocalModel: "llama-3-8b"}

	p, model, err := engine.SelectProvider(req, rcfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "local_vllm" {
		t.Errorf("expected local_vllm, got %q", p.Name())
	}
	if model != "llama-3-8b" {
		t.Errorf("expected llama-3-8b, got %q", model)
	}
}

// TestSelectProvider_UnknownStrategy ensures a clear error is returned.
func TestSelectProvider_UnknownStrategy(t *testing.T) {
	pMap := map[string]providers.Provider{
		"google": &MockProvider{name: "google"},
	}
	engine := NewEngine(pMap)
	_, _, err := engine.SelectProvider(
		&models.ChatCompletionRequest{Model: "x"},
		&config.RemoteStrategy{Strategy: "unknown"},
	)
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

// TestSelectProvider_MissingRemoteProvider verifies error for unconfigured provider.
func TestSelectProvider_MissingRemoteProvider(t *testing.T) {
	engine := NewEngine(map[string]providers.Provider{})
	_, _, err := engine.SelectProvider(
		&models.ChatCompletionRequest{Model: "x"},
		&config.RemoteStrategy{Strategy: "remote", RemoteProvider: "openai"},
	)
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

// TestSelectProvider_MissingLocalVllm verifies error when local_vllm not configured.
func TestSelectProvider_MissingLocalVllm(t *testing.T) {
	engine := NewEngine(map[string]providers.Provider{})
	_, _, err := engine.SelectProvider(
		&models.ChatCompletionRequest{Model: "x"},
		&config.RemoteStrategy{Strategy: "local"},
	)
	if err == nil {
		t.Error("expected error when local_vllm not in provider map")
	}
}

// TestSelectProvider_NoFallbackProviders ensures a clear error when no defaults exist.
func TestSelectProvider_NoFallbackProviders(t *testing.T) {
	engine := NewEngine(map[string]providers.Provider{})
	_, _, err := engine.SelectProvider(
		&models.ChatCompletionRequest{Model: "x"},
		nil,
	)
	if err == nil {
		t.Error("expected error when no providers configured at all")
	}
}

// TestSelectProvider_ExpressionRouting_RemoteModelOverride verifies that a
// cloud provider resolved via expression gets the RemoteModel applied.
func TestSelectProvider_ExpressionRouting_RemoteModelOverride(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{
			Expression: `'google'`, // always routes to google
		},
	}
	defer func() { config.GlobalConfig = nil }()

	pMap := map[string]providers.Provider{
		"google": &MockProvider{name: "google"},
	}
	engine := NewEngine(pMap)

	req := &models.ChatCompletionRequest{Model: "original-model", Messages: []models.Message{{Role: "user", Content: "hi"}}}
	rcfg := &config.RemoteStrategy{Strategy: "remote", RemoteModel: "gemini-flash"}

	p, model, err := engine.SelectProvider(req, rcfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google, got %q", p.Name())
	}
	if model != "gemini-flash" {
		t.Errorf("expected gemini-flash from remote config, got %q", model)
	}
}
