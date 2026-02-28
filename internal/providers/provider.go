package providers

import (
	"context"

	"localrouter/internal/models"
)

// Provider abstracts the underlying generic LLM vendor interface
type Provider interface {
	// Name returns the provider's identifier (e.g. "openai", "google")
	Name() string

	// ChatCompletion performs a standard synchronous request
	ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error)

	// ChatCompletionStream performs a streaming request, returning fragments through streamChan.
	// The implementation should close the channel when finished or return an error if initialization fails.
	ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error
}
