package main

import (
	"fmt"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/providers"
	"agentic-llm-gateway/internal/providers/anthropic"
	"agentic-llm-gateway/internal/providers/google"
	"agentic-llm-gateway/internal/providers/openai"
	"agentic-llm-gateway/internal/router"
	"agentic-llm-gateway/internal/server"
	"agentic-llm-gateway/pkg/logger"
)

func main() {
	cfg, err := config.LoadLocalConfig()
	if err != nil {
		logger.Fatalf("Fatal parsing config: %v", err)
	}

	// Initialize Provider Map and ModelSetter registry concurrently.
	providerMap := make(map[string]providers.Provider)
	modelSetters := make(map[string]config.ModelSetter)

	// Helper: registers a provider in both maps if it implements ModelSetter.
	register := func(name string, p providers.Provider) {
		providerMap[name] = p
		if ms, ok := p.(config.ModelSetter); ok {
			modelSetters[name] = ms
		}
	}

	// OpenAI (generic OpenAI-compatible)
	if pCfg, ok := cfg.Providers["openai"]; ok {
		p := openai.NewProvider("openai", pCfg.APIKey, pCfg.BaseURL, pCfg.DefaultModel)
		register("openai", p)
	}
	// Deepseek (OpenAI-compatible endpoint)
	if pCfg, ok := cfg.Providers["deepseek"]; ok {
		p := openai.NewProvider("deepseek", pCfg.APIKey, pCfg.BaseURL, pCfg.DefaultModel)
		register("deepseek", p)
	}
	// Local vLLM (OpenAI-compatible endpoint)
	if pCfg, ok := cfg.Providers["local_vllm"]; ok {
		p := openai.NewProvider("local_vllm", pCfg.APIKey, pCfg.BaseURL, pCfg.DefaultModel)
		register("local_vllm", p)
	}

	// Anthropic
	if pCfg, ok := cfg.Providers["anthropic"]; ok {
		p := anthropic.NewProvider(pCfg.APIKey, pCfg.DefaultModel)
		register("anthropic", p)
	}
	// Google
	if pCfg, ok := cfg.Providers["google"]; ok {
		p := google.NewProvider(pCfg.APIKey, pCfg.DefaultModel)
		register("google", p)
	}

	// Wire 404 fallback getter into each provider package.
	// The getter reads the current remote strategy so providers need not import config.
	rm := config.NewRemoteManager(cfg.RemoteStrategy.URL, cfg.RemoteStrategy.PollInterval, modelSetters)
	fallbackGetter := func() bool { return rm.GetStrategy().FallbackOn404Enabled() }
	openai.SetFallbackGetter(fallbackGetter)
	anthropic.SetFallbackGetter(fallbackGetter)
	google.SetFallbackGetter(fallbackGetter)

	// Init strategy engine and remote strategy manager.
	engine := router.NewEngine(providerMap)
	rm.Start()

	// Init and start HTTP server.
	srv := server.NewServer(rm, engine)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := srv.Start(addr); err != nil {
		logger.Fatalf("Server stopped: %v", err)
	}
}
