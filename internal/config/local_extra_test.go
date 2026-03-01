package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalConfig_GenerativeRouting_Enabled(t *testing.T) {
	// Setup a temporary valid config file
	data := `
server:
  port: 8080
remote_strategy:
  poll_interval: 10s
  url: "http://example.com"
providers:
  openai:
    api_key: "sk-test"
generative_routing:
  enabled: true
  global_timeout_ms: 1000
  fallback_provider: "openai"
  resolution:
    type: "strict_local_first"
    default_provider: "openai"
  evaluators:
    - name: "test_len"
      type: "builtin_length"
      threshold: 100
`
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte(data))
	tmp.Close()

	os.Setenv("LOCALROUTER_CONFIG_PATH", tmp.Name())
	defer os.Unsetenv("LOCALROUTER_CONFIG_PATH")

	cfg, err := LoadLocalConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if !cfg.GenerativeRouting.Enabled {
		t.Errorf("expected generative routing to be enabled")
	}
	if len(cfg.GenerativeRouting.Evaluators) != 1 {
		t.Errorf("expected 1 evaluator, got %d", len(cfg.GenerativeRouting.Evaluators))
	}
	if GlobalConfig != cfg {
		t.Errorf("expected GlobalConfig to be set to the returned config")
	}
}

func TestLoadLocalConfig_FileDoesNotExist(t *testing.T) {
	// Should create a default config file
	tmpDir, err := os.MkdirTemp("", "config-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "does-not-exist.yaml")
	os.Setenv("LOCALROUTER_CONFIG_PATH", configPath)
	defer os.Unsetenv("LOCALROUTER_CONFIG_PATH")

	cfg, err := LoadLocalConfig()
	if err == nil {
		t.Fatalf("expected error indicating template was generated, got nil")
	}

	// Verify file was actually created
	if _, errStat := os.Stat(configPath); os.IsNotExist(errStat) {
		t.Errorf("expected default config file to be created at %s", configPath)
	}
	_ = cfg

}

func TestLoadLocalConfig_InvalidYAML(t *testing.T) {
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte("invalid\n  yaml:\tfile\n"))
	tmp.Close()

	os.Setenv("LOCALROUTER_CONFIG_PATH", tmp.Name())
	defer os.Unsetenv("LOCALROUTER_CONFIG_PATH")

	_, err = LoadLocalConfig()
	if err == nil {
		t.Error("expected error for invalid yaml")
	}
}
