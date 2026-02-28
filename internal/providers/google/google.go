package google

import (
	"context"
	"fmt"
	"localrouter/internal/models"
)

// Provider implements the Google Gemini API upstream.
type Provider struct {
	apiKey string
}

func NewProvider(apiKey string) *Provider {
	return &Provider{apiKey: apiKey}
}

func (p *Provider) Name() string {
	return "google"
}

func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	// TODO: Map models.ChatCompletionRequest to Gemini GenerateContent API format
	// TODO: POST to https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent
	// TODO: Map Gemini response back to models.ChatCompletionResponse
	return nil, fmt.Errorf("google non-streaming adapter not yet implemented")
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	// TODO: Implement Gemini stream GenerateContent mapping
	return fmt.Errorf("google streaming adapter not yet implemented")
}
