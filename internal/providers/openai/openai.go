package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/httputil"
	"agentic-llm-gateway/pkg/logger"
)

// DefaultModel is the compile-time default model name used when neither the
// request nor the runtime configuration specifies one.
const DefaultModel = "gpt-5"

// Provider implements the OpenAI-compatible upstream provider.
type Provider struct {
	name         string
	apiKey       string
	baseURL      string
	client       *http.Client
	mu           sync.RWMutex
	defaultModel string // runtime-configurable; falls back to DefaultModel const
}

// NewProvider creates a new generic OpenAI-compatible provider instance.
// defaultModel is the initial runtime default; pass an empty string to use the
// compile-time DefaultModel constant.
func NewProvider(name, apiKey, baseURL, defaultModel string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Provider{
		name:         name,
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: defaultModel,
		client:       &http.Client{},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return p.name }

// SetDefaultModel updates the runtime default model name in a thread-safe manner.
// It implements config.ModelSetter so RemoteManager can push live overrides.
func (p *Provider) SetDefaultModel(model string) {
	p.mu.Lock()
	p.defaultModel = model
	p.mu.Unlock()
}

// resolveModel returns reqModel if non-empty, otherwise the runtime default or
// the compile-time DefaultModel constant.
func (p *Provider) resolveModel(reqModel string) string {
	if reqModel != "" {
		return reqModel
	}
	p.mu.RLock()
	m := p.defaultModel
	p.mu.RUnlock()
	if m != "" {
		return m
	}
	return DefaultModel
}

func (p *Provider) postRequest(ctx context.Context, endpoint string, reqBody interface{}) (*http.Response, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s", p.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Error("OpenAI API network request failed", "error", err, "provider", p.name, "endpoint", endpoint)
	}
	return resp, err
}

// ChatCompletion performs a synchronous chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the call is retried once with the compile-time DefaultModel,
// unless the active remote strategy has disabled 404 fallback.
func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Stream = false
	req.Model = p.resolveModel(req.Model)

	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		logger.Warn("OpenAI API 404: model not found, falling back to default",
			"provider", p.name, "attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		return p.chatCompletionOnce(ctx, req)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("provider returned status %d", resp.StatusCode)
		logger.Error("OpenAI API request failed with status", "error", err, "provider", p.name)
		return nil, err
	}

	var completion models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, err
	}
	return &completion, nil
}

// chatCompletionOnce executes a single (non-retried) chat completion request with
// the body already set. Used internally for the 404 fallback retry.
func (p *Provider) chatCompletionOnce(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("provider returned status %d after 404 fallback", resp.StatusCode)
		logger.Error("OpenAI API fallback request failed", "error", err, "provider", p.name, "model", req.Model)
		return nil, err
	}

	var completion models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, err
	}
	return &completion, nil
}

// ChatCompletionStream performs a streaming chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the stream is retried once with the compile-time DefaultModel,
// unless 404 fallback has been disabled via remote config.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	req.Stream = true
	req.Model = p.resolveModel(req.Model)

	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		resp.Body.Close()
		logger.Warn("OpenAI API 404: model not found, falling back to default for stream",
			"provider", p.name, "attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		return p.chatCompletionStreamOnce(ctx, req, streamChan)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("provider returned status %d", resp.StatusCode)
		logger.Error("OpenAI API streaming request failed with status", "error", err, "provider", p.name)
		return err
	}

	go p.pipeStream(ctx, resp, req.Model, streamChan)
	return nil
}

// chatCompletionStreamOnce executes a single (non-retried) streaming request.
func (p *Provider) chatCompletionStreamOnce(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("provider returned status %d after 404 fallback", resp.StatusCode)
		logger.Error("OpenAI API fallback stream request failed", "error", err, "provider", p.name, "model", req.Model)
		return err
	}

	go p.pipeStream(ctx, resp, req.Model, streamChan)
	return nil
}

func (p *Provider) pipeStream(ctx context.Context, resp *http.Response, model string, streamChan chan<- *models.ChatCompletionStreamResponse) {
	defer resp.Body.Close()
	defer close(streamChan)

	err := httputil.ProcessSSEStream(resp.Body, func(data []byte) error {
		var chunk models.ChatCompletionStreamResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case streamChan <- &chunk:
			return nil
		}
	})

	if err != nil && err != context.Canceled {
		logger.Error("OpenAI Stream error", "error", err, "provider", p.name)
	}
}

// shouldFallback consults the active remote strategy to decide whether 404 fallback
// is enabled. Defaults to true when no strategy is present.
// We access the config package via an interface-free helper to avoid circular deps;
// the fallback flag lives on the strategy struct, not on the provider.
func (p *Provider) shouldFallback(_ context.Context) bool {
	// Fallback is opt-out: absence of the field means enabled.
	// We read the flag from the package-level helper to avoid importing config.
	return globalFallbackOn404()
}

// globalFallbackOn404 is a package-level variable that can be overridden in tests
// or set by wiring code. By default it reads from the active RemoteStrategy via a
// registered getter to avoid circular imports with the config package.
//
// The real wiring is done by setting fallbackGetter in main via SetFallbackGetter.
var fallbackGetter func() bool

// SetFallbackGetter registers a function that reports whether 404 fallback is
// currently enabled. Call this once during startup from cmd/server/main.go.
func SetFallbackGetter(fn func() bool) { fallbackGetter = fn }

func globalFallbackOn404() bool {
	if fallbackGetter != nil {
		return fallbackGetter()
	}
	return true // safe default: enable fallback when no getter is wired
}
