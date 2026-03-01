package config

import (
	"agentic-llm-gateway/pkg/logger"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server            ServerConfig              `yaml:"server"`
	RemoteStrategy    RemoteStrategyConfig      `yaml:"remote_strategy"`
	Providers         map[string]ProviderConfig `yaml:"providers"`
	GenerativeRouting *GenerativeRoutingConfig  `yaml:"generative_routing,omitempty"`
}

// GenerativeRoutingConfig configures the smart routing based on generative models
type GenerativeRoutingConfig struct {
	Enabled          bool                     `yaml:"enabled"`
	GlobalTimeoutMs  int                      `yaml:"global_timeout_ms"`
	FallbackProvider string                   `yaml:"fallback_provider"`
	Evaluators       []EvaluatorConfig        `yaml:"evaluators"`
	Resolution       ResolutionStrategyConfig `yaml:"resolution_strategy"`
}

// EvaluatorConfig configures a single intent dimension evaluator
type EvaluatorConfig struct {
	Name           string         `yaml:"name"`
	Type           string         `yaml:"type"`               // e.g. "llm_api", "builtin"
	Protocol       string         `yaml:"protocol,omitempty"` // "ollama" (default) or "openai"
	Endpoint       string         `yaml:"endpoint,omitempty"`
	Model          string         `yaml:"model,omitempty"`
	HistoryRounds  int            `yaml:"history_rounds,omitempty"`
	TimeoutMs      int            `yaml:"timeout_ms,omitempty"`
	LogitBias      map[string]int `yaml:"logit_bias,omitempty"`
	PromptTemplate string         `yaml:"prompt_template,omitempty"`
	Threshold      int            `yaml:"threshold,omitempty"`
}

// ResolutionStrategyConfig configures how to make routing decision based on eval vectors
type ResolutionStrategyConfig struct {
	Type            string                 `yaml:"type"` // e.g. "dynamic_expression"
	Rules           []ResolutionRuleConfig `yaml:"rules,omitempty"`
	DefaultProvider string                 `yaml:"default_provider"`
}

// ResolutionRuleConfig determines condition to hit specific target provider
type ResolutionRuleConfig struct {
	Condition      string `yaml:"condition"`
	TargetProvider string `yaml:"target_provider"`
}

// ServerConfig configures the HTTP server
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// RemoteStrategyConfig configures the remote JSON strategy origin
type RemoteStrategyConfig struct {
	URL          string        `yaml:"url"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Expression   string        `yaml:"expression"`
}

// ProviderConfig configures a specific upstream provider
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

const DefaultConfigTemplate = `server:
  port: 8080
  host: "127.0.0.1"
remote_strategy:
  url: "https://your-config-domain.com/strategy.json"
  poll_interval: 60s
  expression: ""
providers:
  openai:
    api_key: "sk-..."
  anthropic:
    api_key: "sk-ant-..."
  google:
    api_key: "AIza..."
  deepseek:
    api_key: "sk-..."
    base_url: "https://api.deepseek.com/v1"
  local_vllm:
    base_url: "http://192.168.1.100:8000/v1"
`

var GlobalConfig *Config

// LoadLocalConfig loads configuration from LOCALROUTER_CONFIG_PATH or ~/.config/agentic-llm-gateway/config.yaml
// If the configuration file doesn't exist, it creates a template for the user.
func LoadLocalConfig() (*Config, error) {
	configPath := os.Getenv("LOCALROUTER_CONFIG_PATH")
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		configPath = filepath.Join(home, ".config", "agentic-llm-gateway", "config.yaml")
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Warnf("Config file missing at %s, creating default template...", configPath)
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}
		if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate), 0644); err != nil {
			return nil, fmt.Errorf("failed to write default config template: %w", err)
		}
		return nil, fmt.Errorf("generated default config at %s. Please update it and restart", configPath)
	}

	// Read and parse
	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var conf Config
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	GlobalConfig = &conf

	return &conf, nil
}
