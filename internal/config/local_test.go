package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalConfig_GenerativeRouting(t *testing.T) {
	// Create a temporary config file
	content := `
server:
  port: 8080
  host: "127.0.0.1"
remote_strategy:
  url: "https://your-config-domain.com/strategy.json"
  poll_interval: 60s
  expression: ""
providers:
  openai:
    api_key: "sk-..."
generative_routing:
  enabled: true
  global_timeout_ms: 100
  fallback_provider: "openai"
  evaluators:
    - name: "complexity"
      type: "llm_api"
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen-0.6b"
      history_rounds: 1
      timeout_ms: 60
      logit_bias:
        "15": 100
        "16": 100
      prompt_template: "test"
    - name: "length_check"
      type: "builtin"
      threshold: 50
  resolution_strategy:
    type: "dynamic_expression"
    rules:
      - condition: "complexity == 0 && length_check < 50"
        target_provider: "local-qwen-0.5b"
    default_provider: "openai"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mock config: %v", err)
	}

	t.Setenv("LOCALROUTER_CONFIG_PATH", configPath)

	conf, err := LoadLocalConfig()
	if err != nil {
		t.Fatalf("LoadLocalConfig failed: %v", err)
	}

	if conf.GenerativeRouting == nil {
		t.Fatal("GenerativeRouting should not be nil")
	}

	if !conf.GenerativeRouting.Enabled {
		t.Errorf("expected GenerativeRouting.Enabled to be true, got %v", conf.GenerativeRouting.Enabled)
	}

	if conf.GenerativeRouting.GlobalTimeoutMs != 100 {
		t.Errorf("expected GlobalTimeoutMs to be 100, got %d", conf.GenerativeRouting.GlobalTimeoutMs)
	}

	if len(conf.GenerativeRouting.Evaluators) != 2 {
		t.Fatalf("expected 2 evaluators, got %d", len(conf.GenerativeRouting.Evaluators))
	}

	eval1 := conf.GenerativeRouting.Evaluators[0]
	if eval1.Name != "complexity" || eval1.Type != "llm_api" || eval1.LogitBias["15"] != 100 {
		t.Errorf("evaluator 1 parsed incorrectly: %+v", eval1)
	}

	eval2 := conf.GenerativeRouting.Evaluators[1]
	if eval2.Name != "length_check" || eval2.Type != "builtin" || eval2.Threshold != 50 {
		t.Errorf("evaluator 2 parsed incorrectly: %+v", eval2)
	}

	res := conf.GenerativeRouting.Resolution
	if res.Type != "dynamic_expression" || len(res.Rules) != 1 || res.DefaultProvider != "openai" {
		t.Errorf("resolution strategy parsed incorrectly: %+v", res)
	}
}

func TestLoadLocalConfig_NoGenerativeRouting(t *testing.T) {
	// Create a temporary config file without generative_routing
	content := `
server:
  port: 8080
  host: "127.0.0.1"
remote_strategy:
  url: "https://your-config-domain.com/strategy.json"
  poll_interval: 60s
  expression: ""
providers:
  openai:
    api_key: "sk-..."
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mock config: %v", err)
	}

	t.Setenv("LOCALROUTER_CONFIG_PATH", configPath)

	conf, err := LoadLocalConfig()
	if err != nil {
		t.Fatalf("LoadLocalConfig failed: %v", err)
	}

	// It should be successfully parsed, leaving GenerativeRouting as nil
	if conf.GenerativeRouting != nil {
		t.Fatalf("expected GenerativeRouting to be nil if not present in config, got %+v", conf.GenerativeRouting)
	}
}
