package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/logger"
)

// DefaultModel is the compile-time default model name used when neither the
// request nor the runtime configuration specifies one.
const DefaultModel = "gemini-3.0-flash-preview"

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

// Provider implements the Google Gemini REST API.
type Provider struct {
	apiKey       string
	baseURL      string
	client       *http.Client
	mu           sync.RWMutex
	defaultModel string // runtime-configurable; falls back to DefaultModel const
}

// NewProvider creates a new Google Gemini provider instance.
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
func (p *Provider) Name() string { return "google" }

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

// --- Google Gemini API structures ---

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

func mapRequest(req *models.ChatCompletionRequest) *geminiRequest {
	greq := &geminiRequest{}

	for _, m := range req.Messages {
		role := strings.ToLower(m.Role)
		// Gemini only supports "user" and "model"
		if role == "assistant" {
			role = "model"
		} else if role == "system" {
			role = "user" // simplification for older Gemini APIs
		} else {
			role = "user"
		}

		greq.Contents = append(greq.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	return greq
}

// redactKey replaces occurrences of the API key in s with "***" to prevent
// key leakage in log output.
func (p *Provider) redactKey(s string) string {
	if p.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, p.apiKey, "***")
}

// ChatCompletion performs a synchronous chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the call is retried once with the compile-time DefaultModel,
// unless 404 fallback has been disabled via remote config.
func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Model = p.resolveModel(req.Model)
	greq := mapRequest(req)

	resp, err := p.doGenerateContent(ctx, req.Model, greq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		logger.Warn("Google API 404: model not found, falling back to default",
			"attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		return p.chatCompletionOnce(ctx, req)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		safeErr := p.redactKey(fmt.Sprintf("google api error %d: %s", resp.StatusCode, string(body)))
		logger.Error("Google API request failed with status", "error", safeErr, "model", req.Model)
		return nil, fmt.Errorf("%s", safeErr)
	}

	return p.decodeResponse(resp, req.Model)
}

func (p *Provider) chatCompletionOnce(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	greq := mapRequest(req)
	resp, err := p.doGenerateContent(ctx, req.Model, greq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		safeErr := p.redactKey(fmt.Sprintf("google api error %d: %s after 404 fallback", resp.StatusCode, string(body)))
		logger.Error("Google API fallback request failed", "error", safeErr, "model", req.Model)
		return nil, fmt.Errorf("%s", safeErr)
	}

	return p.decodeResponse(resp, req.Model)
}

func (p *Provider) doGenerateContent(ctx context.Context, model string, greq *geminiRequest) (*http.Response, error) {
	data, err := json.Marshal(greq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s:generateContent?key=%s", p.baseURL, model, p.apiKey)
	hreq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(hreq)
	if err != nil {
		safeErr := p.redactKey(err.Error())
		logger.Error("Google API network request failed", "error", safeErr, "model", model)
		return nil, fmt.Errorf("%s", safeErr)
	}
	return resp, nil
}

func (p *Provider) decodeResponse(resp *http.Response, model string) (*models.ChatCompletionResponse, error) {
	var gresp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gresp); err != nil {
		return nil, err
	}

	content := ""
	if len(gresp.Candidates) > 0 && len(gresp.Candidates[0].Content.Parts) > 0 {
		content = gresp.Candidates[0].Content.Parts[0].Text
	}

	return &models.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{{
			Index:        0,
			Message:      models.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
	}, nil
}

// ChatCompletionStream performs a streaming chat completion request.
// If req.Model is empty the provider default is used.
// On HTTP 404 the stream is retried once with the compile-time DefaultModel,
// unless 404 fallback has been disabled via remote config.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	req.Model = p.resolveModel(req.Model)
	greq := mapRequest(req)

	resp, err := p.doStreamGenerateContent(ctx, req.Model, greq)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound && req.Model != DefaultModel && p.shouldFallback(ctx) {
		resp.Body.Close()
		logger.Warn("Google API 404: model not found, falling back to default for stream",
			"attempted_model", req.Model, "fallback_model", DefaultModel)
		req.Model = DefaultModel
		return p.chatCompletionStreamOnce(ctx, req, streamChan)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		safeErr := p.redactKey(fmt.Sprintf("google api error %d: %s", resp.StatusCode, string(body)))
		logger.Error("Google API streaming request failed with status", "error", safeErr, "model", req.Model)
		return fmt.Errorf("%s", safeErr)
	}

	go p.pipeStream(ctx, resp, req.Model, streamChan)
	return nil
}

func (p *Provider) chatCompletionStreamOnce(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	greq := mapRequest(req)
	resp, err := p.doStreamGenerateContent(ctx, req.Model, greq)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		safeErr := p.redactKey(fmt.Sprintf("google api error %d: %s after 404 fallback", resp.StatusCode, string(body)))
		logger.Error("Google API fallback stream request failed", "error", safeErr, "model", req.Model)
		return fmt.Errorf("%s", safeErr)
	}

	go p.pipeStream(ctx, resp, req.Model, streamChan)
	return nil
}

func (p *Provider) doStreamGenerateContent(ctx context.Context, model string, greq *geminiRequest) (*http.Response, error) {
	data, err := json.Marshal(greq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, model, p.apiKey)
	hreq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(hreq)
	if err != nil {
		safeErr := p.redactKey(err.Error())
		logger.Error("Google API streaming network request failed", "error", safeErr, "model", model)
		return nil, fmt.Errorf("%s", safeErr)
	}
	return resp, nil
}

func (p *Provider) pipeStream(ctx context.Context, resp *http.Response, model string, streamChan chan<- *models.ChatCompletionStreamResponse) {
	defer resp.Body.Close()
	defer close(streamChan)

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		payload := bytes.TrimPrefix(line, []byte("data: "))
		if bytes.Equal(payload, []byte("[DONE]")) {
			break
		}

		var gresp geminiResponse
		if err := json.Unmarshal(payload, &gresp); err != nil {
			continue
		}

		if len(gresp.Candidates) > 0 && len(gresp.Candidates[0].Content.Parts) > 0 {
			chunk := &models.ChatCompletionStreamResponse{
				ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
			}
			chunk.Choices = []struct {
				Index int `json:"index"`
				Delta struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{{
				Index: 0,
				Delta: struct {
					Role    string `json:"role,omitempty"`
					Content string `json:"content,omitempty"`
				}{
					Content: gresp.Candidates[0].Content.Parts[0].Text,
				},
			}}

			select {
			case <-ctx.Done():
				return
			case streamChan <- chunk:
			}
		}
	}

	if err := scanner.Err(); err != nil && err != context.Canceled {
		safeErr := p.redactKey(err.Error())
		logger.Error("Google Stream error", "error", safeErr)
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
