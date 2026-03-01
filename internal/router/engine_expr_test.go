package router

import (
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
)

func TestSelectProvider_NilRemoteConfig_Google(t *testing.T) {
	engine := NewEngine(map[string]providers.Provider{
		"google": &MockProvider{name: "google"},
	})
	p, model, err := engine.SelectProvider(&models.ChatCompletionRequest{Model: "default-model"}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "google" || model != "default-model" {
		t.Errorf("expected google and default-model, got %q, %q", p.Name(), model)
	}
}

func TestSelectProvider_NilRemoteConfig_LocalVllm(t *testing.T) {
	engine := NewEngine(map[string]providers.Provider{
		"local_vllm": &MockProvider{name: "local_vllm"},
	})
	p, model, err := engine.SelectProvider(&models.ChatCompletionRequest{Model: "default-model"}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "local_vllm" || model != "default-model" {
		t.Errorf("expected local_vllm and default-model, got %q, %q", p.Name(), model)
	}
}

func TestSelectProvider_ExprCompileError(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{Expression: "invalid + )"},
	}
	defer func() { config.GlobalConfig = nil }()

	engine := NewEngine(map[string]providers.Provider{
		"google": &MockProvider{name: "google"},
	})
	// Will log error and fall through to default remote routing
	p, _, err := engine.SelectProvider(&models.ChatCompletionRequest{}, &config.RemoteStrategy{Strategy: "remote"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google fallback, got %q", p.Name())
	}
}

func TestSelectProvider_ExprRunError(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{Expression: "Req.UnknownField == 1"},
	}
	defer func() { config.GlobalConfig = nil }()

	engine := NewEngine(map[string]providers.Provider{"google": &MockProvider{name: "google"}})
	p, _, err := engine.SelectProvider(&models.ChatCompletionRequest{}, &config.RemoteStrategy{Strategy: "remote"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google fallback, got %q", p.Name())
	}
}

func TestSelectProvider_ExprWrongType(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{Expression: "1 + 1"},
	}
	defer func() { config.GlobalConfig = nil }()

	engine := NewEngine(map[string]providers.Provider{"google": &MockProvider{name: "google"}})
	p, _, err := engine.SelectProvider(&models.ChatCompletionRequest{}, &config.RemoteStrategy{Strategy: "remote"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google fallback, got %q", p.Name())
	}
}

func TestSelectProvider_ExprUnknownProvider(t *testing.T) {
	config.GlobalConfig = &config.Config{
		RemoteStrategy: config.RemoteStrategyConfig{Expression: "'unknown_provider'"},
	}
	defer func() { config.GlobalConfig = nil }()

	engine := NewEngine(map[string]providers.Provider{"google": &MockProvider{name: "google"}})
	p, _, err := engine.SelectProvider(&models.ChatCompletionRequest{}, &config.RemoteStrategy{Strategy: "remote"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.Name() != "google" {
		t.Errorf("expected google fallback, got %q", p.Name())
	}
}
