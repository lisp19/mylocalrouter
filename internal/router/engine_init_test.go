package router

import (
	"testing"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/providers"
)

func TestNewEngine_LoadsEvaluators(t *testing.T) {
	config.GlobalConfig = &config.Config{
		GenerativeRouting: &config.GenerativeRoutingConfig{
			Enabled: true,
			Evaluators: []config.EvaluatorConfig{
				{Name: "len", Type: "builtin"},
				{Name: "api", Type: "llm_api", Protocol: "ollama", Endpoint: "http://localhost"},
				{Name: "log", Type: "llm_logprob_api", Protocol: "openai", Endpoint: "http://localhost"},
				{Name: "bad", Type: "unknown"},
			},
		},
	}
	defer func() { config.GlobalConfig = nil }()

	engine := NewEngine(map[string]providers.Provider{})
	defEng, ok := engine.(*defaultEngine)
	if !ok {
		t.Fatal("expected engine to be *defaultEngine")
	}

	// Valid configs should be loaded, invalid skipped
	if len(defEng.evaluators) != 3 {
		t.Errorf("expected 3 valid evaluators loaded, got %d", len(defEng.evaluators))
	}
}
