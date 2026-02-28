package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the root configuration structure
type Config struct {
	Server         ServerConfig              `yaml:"server"`
	RemoteStrategy RemoteStrategyConfig      `yaml:"remote_strategy"`
	Providers      map[string]ProviderConfig `yaml:"providers"`
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

// LoadLocalConfig loads configuration from LOCALROUTER_CONFIG_PATH or ~/.config/localrouter/config.yaml
// If the configuration file doesn't exist, it creates a template for the user.
func LoadLocalConfig() (*Config, error) {
	configPath := os.Getenv("LOCALROUTER_CONFIG_PATH")
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		configPath = filepath.Join(home, ".config", "localrouter", "config.yaml")
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("Config file missing at %s, creating default template...", configPath)
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

	return &conf, nil
}
