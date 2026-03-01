package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/httputil"
	"agentic-llm-gateway/pkg/logger"
)

// DefaultModel is the compile-time default model name used when neither the
// request nor the runtime configuration specifies one.
const DefaultModel = "claude-3-5-haiku-20241022"

const defaultBaseURL = "https://api.anthropic.com/v1"

// Provider implements the Anthropic Messages API.
type Provider struct {
	apiKey       string
	baseURL      string
	client       *http.Client
	mu           sync.RWMutex
	defaultModel string // runtime-configurable; falls back to DefaultModel const
}

// NewProvider creates a new Anthropic provider instance.
// defaultModel is the initial runtime default; pass an empty string to use the
// compile-time DefaultModel constant.
func NewProvider(apiKey, defaultModel string) *Provider {
	return &Provider{
		apiKey:       apiKey,
		baseURL:      defaultBaseURL,
		defaultModel: defaultModel,
		client:       &http.Client{},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "anthropic" }

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

// --- Anthropic API structures ---

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
}

func mapRequest(req *models.ChatCompletionRequest) *anthropicRequest {
	areq := &anthropicRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if areq.MaxTokens == 0 {
		areq.MaxTokens = 4096 // Claude requires max_tokens
	}

	for _, m := range req.Messages {
		if strings.ToLower(m.Role) == "system" {
			areq.System = m.Content
			continue
		}

		role := strings.ToLower(m.Role)
		if role != "user" && role != "assistant" {
			role = "user"
		}

		areq.Messages = append(areq.Messages, anthropicMessage{
			Role:    role,
			Content: m.Content,
		})
	}
	return areq
}

func (p *Provider) doRequest(ctx context.Context, endpoint string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+endpoint, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Error("Anthropic API network request failed", "error", err, "endpoint", endpoint)
	}
	return resp, err
}

// ChatCompletion performs a synchronous chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the call is retried once with the compile-time DefaultModel,
// unless 404 fallback has been disabled via remote config.
func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Stream = false
	req.Model = p.resolveModel(req.Model)
	areq := mapRequest(req)

	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		logger.Warn("Anthropic API 404: model not found, falling back to default",
			"attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		areq = mapRequest(req)
		return p.chatCompletionOnce(ctx, areq)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("anthropic api error: status %d", resp.StatusCode)
		logger.Error("Anthropic API request failed with status", "error", err)
		return nil, err
	}

	return p.decodeResponse(resp)
}

func (p *Provider) chatCompletionOnce(ctx context.Context, areq *anthropicRequest) (*models.ChatCompletionResponse, error) {
	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("anthropic api error: status %d after 404 fallback", resp.StatusCode)
		logger.Error("Anthropic API fallback request failed", "error", err, "model", areq.Model)
		return nil, err
	}

	return p.decodeResponse(resp)
}

func (p *Provider) decodeResponse(resp *http.Response) (*models.ChatCompletionResponse, error) {
	var aresp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&aresp); err != nil {
		return nil, err
	}

	content := ""
	if len(aresp.Content) > 0 {
		content = aresp.Content[0].Text
	}

	return &models.ChatCompletionResponse{
		ID:      aresp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   aresp.Model,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{{
			Index: 0,
			Message: models.Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: "stop",
		}},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     aresp.Usage.InputTokens,
			CompletionTokens: aresp.Usage.OutputTokens,
			TotalTokens:      aresp.Usage.InputTokens + aresp.Usage.OutputTokens,
		},
	}, nil
}

// ChatCompletionStream performs a streaming chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the stream is retried once with the compile-time DefaultModel,
// unless 404 fallback has been disabled via remote config.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	req.Stream = true
	req.Model = p.resolveModel(req.Model)
	areq := mapRequest(req)

	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		resp.Body.Close()
		logger.Warn("Anthropic API 404: model not found, falling back to default for stream",
			"attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		areq = mapRequest(req)
		return p.chatCompletionStreamOnce(ctx, areq, req.Model, streamChan)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("anthropic api error: status %d", resp.StatusCode)
		logger.Error("Anthropic API streaming request failed with status", "error", err)
		return err
	}

	go p.pipeStream(ctx, resp, req.Model, streamChan)
	return nil
}

func (p *Provider) chatCompletionStreamOnce(ctx context.Context, areq *anthropicRequest, model string, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("anthropic api error: status %d after 404 fallback", resp.StatusCode)
		logger.Error("Anthropic API fallback stream request failed", "error", err, "model", areq.Model)
		return err
	}

	go p.pipeStream(ctx, resp, model, streamChan)
	return nil
}

func (p *Provider) pipeStream(ctx context.Context, resp *http.Response, model string, streamChan chan<- *models.ChatCompletionStreamResponse) {
	defer resp.Body.Close()
	defer close(streamChan)

	err := httputil.ProcessSSEStream(resp.Body, func(data []byte) error {
		var event anthropicStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil // ignore decode errors on partial chunks
		}

		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			chunk := &models.ChatCompletionStreamResponse{
				ID:      fmt.Sprintf("chatcmpl-claude-%d", time.Now().UnixNano()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
			}

			delta := struct {
				Role    string `json:"role,omitempty"`
				Content string `json:"content,omitempty"`
			}{
				Content: event.Delta.Text,
			}

			chunk.Choices = []struct {
				Index int `json:"index"`
				Delta struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{{Index: 0, Delta: delta}}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case streamChan <- chunk:
			}
		}
		return nil
	})

	if err != nil && err != context.Canceled {
		logger.Error("Anthropic Stream error", "error", err)
	}
}

// shouldFallback reports whether 404 model fallback is currently enabled.
func (p *Provider) shouldFallback(_ context.Context) bool {
	return globalFallbackOn404()
}

var fallbackGetter func() bool

// SetFallbackGetter registers a function that reports whether 404 fallback is
// currently enabled. Call this once during startup from cmd/server/main.go.
func SetFallbackGetter(fn func() bool) { fallbackGetter = fn }

func globalFallbackOn404() bool {
	if fallbackGetter != nil {
		return fallbackGetter()
	}
	return true // safe default
}
