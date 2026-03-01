package config

import (
	"agentic-llm-gateway/pkg/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// ModelSetter is implemented by any provider that supports runtime model name updates.
// RemoteManager uses this interface to push per-provider model overrides after each
// successful config fetch, keeping provider packages decoupled from this package.
type ModelSetter interface {
	SetDefaultModel(model string)
}

// RemoteStrategy represents the data structure returned by the remote origin.
type RemoteStrategy struct {
	Strategy       string            `json:"strategy"`        // "local" or "remote"
	LocalModel     string            `json:"local_model"`     // e.g., "qwen-35b-awq"
	RemoteProvider string            `json:"remote_provider"` // e.g., "google", "openai"
	RemoteModel    string            `json:"remote_model"`    // e.g., "gemini-3.0-flash-preview"
	ProviderModels map[string]string `json:"provider_models"` // per-provider model overrides; empty values are ignored
	FallbackOn404  *bool             `json:"fallback_on_404"` // if non-nil, overrides per-provider 404 fallback behaviour
	UpdatedAt      string            `json:"updated_at"`
}

// FallbackOn404Enabled reports whether the remote strategy enables 404 model fallback.
// Returns true (the safe default) when the field is absent from the remote payload.
func (rs *RemoteStrategy) FallbackOn404Enabled() bool {
	if rs == nil || rs.FallbackOn404 == nil {
		return true
	}
	return *rs.FallbackOn404
}

// RemoteManager handles fetching and providing thread-safe access to the remote strategy.
type RemoteManager struct {
	strategy  atomic.Value // underlying type is *RemoteStrategy
	url       string
	interval  time.Duration
	client    *http.Client
	providers map[string]ModelSetter // keyed by provider name, e.g. "openai", "google"
}

// NewRemoteManager initialises a new RemoteManager.
// providers is an optional map of ModelSetter implementations; pass nil if not needed.
func NewRemoteManager(url string, interval time.Duration, providers map[string]ModelSetter) *RemoteManager {
	rm := &RemoteManager{
		url:       url,
		interval:  interval,
		providers: providers,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	// Initialise with an empty strategy to avoid nil dereferences before the first fetch.
	rm.strategy.Store(&RemoteStrategy{})
	return rm
}

// Start begins the polling goroutine, executing an initial fetch immediately.
func (rm *RemoteManager) Start() {
	// Execute immediately once.
	if err := rm.fetch(); err != nil {
		logger.Error("RemoteConfig initial fetch failed", "error", err)
	}

	go func() {
		ticker := time.NewTicker(rm.interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := rm.fetch(); err != nil {
				logger.Error("RemoteConfig fetch error", "error", err)
			}
		}
	}()
}

func (rm *RemoteManager) fetch() error {
	if rm.url == "" {
		return fmt.Errorf("remote strategy URL is empty")
	}

	req, err := http.NewRequest("GET", rm.url, nil)
	if err != nil {
		return err
	}

	resp, err := rm.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var strategy RemoteStrategy
	if err := json.Unmarshal(body, &strategy); err != nil {
		return err
	}

	rm.strategy.Store(&strategy)
	logger.Info("RemoteConfig strategy updated",
		"strategy", strategy.Strategy,
		"local_model", strategy.LocalModel,
		"remote_provider", strategy.RemoteProvider,
		"remote_model", strategy.RemoteModel,
		"provider_models_count", len(strategy.ProviderModels),
	)

	// Push per-provider model overrides; skip empty values to preserve provider defaults.
	rm.applyProviderModels(strategy.ProviderModels)

	return nil
}

// applyProviderModels propagates non-empty model names from the remote config to the
// registered ModelSetter implementations. Empty values are intentionally skipped so
// that providers retain their current (or compile-time default) model names.
func (rm *RemoteManager) applyProviderModels(models map[string]string) {
	if len(models) == 0 || len(rm.providers) == 0 {
		return
	}
	for name, model := range models {
		if model == "" {
			continue // empty → no-op; do not clear an existing override
		}
		if setter, ok := rm.providers[name]; ok {
			setter.SetDefaultModel(model)
			logger.Info("RemoteConfig updated provider default model", "provider", name, "model", model)
		}
	}
}

// GetStrategy returns the latest remote strategy atomically.
func (rm *RemoteManager) GetStrategy() *RemoteStrategy {
	val := rm.strategy.Load()
	if val == nil {
		return &RemoteStrategy{}
	}
	return val.(*RemoteStrategy)
}
