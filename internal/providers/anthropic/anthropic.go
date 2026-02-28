package anthropic

import (
	"context"
	"fmt"
	"localrouter/internal/models"
)

// Provider implements the Anthropic Claude API upstream.
type Provider struct {
	apiKey string
}

func NewProvider(apiKey string) *Provider {
	return &Provider{apiKey: apiKey}
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	// TODO: Map models.ChatCompletionRequest to Anthropic's Messages API format
	// TODO: Send POST to https://api.anthropic.com/v1/messages
	// TODO: Map Anthropic response back to models.ChatCompletionResponse
	return nil, fmt.Errorf("anthropic non-streaming adapter not yet implemented")
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	// TODO: Implement Anthropic SSE streaming mapping
	return fmt.Errorf("anthropic streaming adapter not yet implemented")
}
