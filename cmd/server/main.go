package main

import (
	"fmt"
	"localrouter/pkg/logger"

	"localrouter/internal/config"
	"localrouter/internal/providers"
	"localrouter/internal/providers/anthropic"
	"localrouter/internal/providers/google"
	"localrouter/internal/providers/openai"
	"localrouter/internal/router"
	"localrouter/internal/server"
)

func main() {
	cfg, err := config.LoadLocalConfig()
	if err != nil {
		logger.Fatalf("Fatal parsing config: %v", err)
	}

	// Initialize Provider Map
	providerMap := make(map[string]providers.Provider)

	// OpenAI (Generic fallback if no sub-provider)
	if pCfg, ok := cfg.Providers["openai"]; ok {
		providerMap["openai"] = openai.NewProvider("openai", pCfg.APIKey, pCfg.BaseURL)
	}
	// Deepseek
	if pCfg, ok := cfg.Providers["deepseek"]; ok {
		providerMap["deepseek"] = openai.NewProvider("deepseek", pCfg.APIKey, pCfg.BaseURL)
	}
	// Local vLLM (uses OpenAI adapter)
	if pCfg, ok := cfg.Providers["local_vllm"]; ok {
		providerMap["local_vllm"] = openai.NewProvider("local_vllm", pCfg.APIKey, pCfg.BaseURL)
	}

	// Anthropic
	if pCfg, ok := cfg.Providers["anthropic"]; ok {
		providerMap["anthropic"] = anthropic.NewProvider(pCfg.APIKey)
	}
	// Google
	if pCfg, ok := cfg.Providers["google"]; ok {
		providerMap["google"] = google.NewProvider(pCfg.APIKey)
	}

	// Init strategy engine
	engine := router.NewEngine(providerMap)

	// Init remote strategy manager
	rm := config.NewRemoteManager(cfg.RemoteStrategy.URL, cfg.RemoteStrategy.PollInterval)
	rm.Start()

	// Init Server
	srv := server.NewServer(rm, engine)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := srv.Start(addr); err != nil {
		logger.Fatalf("Server stopped: %v", err)
	}
}
