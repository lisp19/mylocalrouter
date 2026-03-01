package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"agentic-llm-gateway/pkg/logger"
	"net/http"
	"strings"
	"time"

	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/httputil"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewProvider(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string {
	return "anthropic"
}

// anthropic structures
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

func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Stream = false
	areq := mapRequest(req)

	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("anthropic api error: status %d", resp.StatusCode)
		logger.Error("Anthropic API network request failed with status", "error", err)
		return nil, err
	}

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

func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	req.Stream = true
	areq := mapRequest(req)

	resp, err := p.doRequest(ctx, "/messages", areq)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("anthropic api error: status %d", resp.StatusCode)
		logger.Error("Anthropic API streaming network request failed with status", "error", err)
		return err
	}

	go func() {
		defer resp.Body.Close()
		defer close(streamChan)

		err := httputil.ProcessSSEStream(resp.Body, func(data []byte) error {
			var event anthropicStreamEvent
			if err := json.Unmarshal(data, &event); err != nil {
				return err // Ignore decode errors on partial chunks
			}

			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				var chunk models.ChatCompletionStreamResponse
				chunk.ID = fmt.Sprintf("chatcmpl-claude-%d", time.Now().UnixNano())
				chunk.Object = "chat.completion.chunk"
				chunk.Created = time.Now().Unix()
				chunk.Model = req.Model

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
				}{{
					Index: 0,
					Delta: delta,
				}}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case streamChan <- &chunk:
				}
			}
			return nil
		})

		if err != nil && err != context.Canceled {
			logger.Error("Anthropic Stream error", "error", err)
		}
	}()

	return nil
}
