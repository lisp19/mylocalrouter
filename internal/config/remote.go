package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// RemoteStrategy represents the data structure returned by the remote origin.
type RemoteStrategy struct {
	Strategy    string `json:"strategy"`     // "local" or "remote"
	LocalModel  string `json:"local_model"`  // e.g., "qwen-35b-awq"
	RemoteModel string `json:"remote_model"` // e.g., "gemini-3-flash"
	UpdatedAt   string `json:"updated_at"`
}

// RemoteManager handles fetching and providing thread-safe access to the remote strategy
type RemoteManager struct {
	strategy atomic.Value // underlying type is *RemoteStrategy
	url      string
	interval time.Duration
	client   *http.Client
}

// NewRemoteManager initializes a new RemoteManager.
func NewRemoteManager(url string, interval time.Duration) *RemoteManager {
	rm := &RemoteManager{
		url:      url,
		interval: interval,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	// Init with an empty strategy to avoid nil exceptions
	rm.strategy.Store(&RemoteStrategy{})
	return rm
}

// Start begins the polling goroutine. Blocks until the first successful fetch or handles it asynchronously.
func (rm *RemoteManager) Start() {
	// Execute immediately once
	if err := rm.fetch(); err != nil {
		log.Printf("[RemoteConfig] Initial fetch failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(rm.interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := rm.fetch(); err != nil {
				log.Printf("[RemoteConfig] Fetch error: %v", err)
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
	log.Printf("[RemoteConfig] Strategy updated at %s: strategy=%s, local=%s, remote=%s",
		time.Now().Format(time.RFC3339), strategy.Strategy, strategy.LocalModel, strategy.RemoteModel)

	return nil
}

// GetStrategy returns the latest remote strategy atomically.
func (rm *RemoteManager) GetStrategy() *RemoteStrategy {
	val := rm.strategy.Load()
	if val == nil {
		return &RemoteStrategy{}
	}
	return val.(*RemoteStrategy)
}
