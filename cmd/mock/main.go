package main

import (
	"context"
	"fmt"
	"agentic-llm-gateway/pkg/logger"
	"time"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/internal/providers"
	"agentic-llm-gateway/internal/router"
	"agentic-llm-gateway/internal/server"
)

// MockProvider generates fake OpenAI streaming responses for testing
type MockProvider struct{}

func (p *MockProvider) Name() string {
	return "mock"
}

func (p *MockProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	time.Sleep(500 * time.Millisecond) // Simulate network latency
	return &models.ChatCompletionResponse{
		ID:      "chatcmpl-mock",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{{
			Index: 0,
			Message: models.Message{
				Role:    "assistant",
				Content: "Hello! this is a mock non-streaming response.",
			},
			FinishReason: "stop",
		}},
	}, nil
}

func (p *MockProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	go func() {
		defer close(streamChan)
		words := []string{"Hello!", " I", " am", " a", " mock", " streaming", " assistant.", " How", " can", " I", " help?"}
		
		for i, word := range words {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(100 * time.Millisecond) // chunk delay
				var finishReason *string
				if i == len(words)-1 {
					reason := "stop"
					finishReason = &reason
				}
				
				streamChan <- &models.ChatCompletionStreamResponse{
					ID:      "chatcmpl-mock-stream",
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []struct {
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
						}{Role: "assistant", Content: word},
						FinishReason: finishReason,
					}},
				}
			}
		}
	}()
	return nil
}

func main() {
	// Initialize mock tools
	providerMap := map[string]providers.Provider{
		"mock": &MockProvider{},
	}

	engine := router.NewEngine(providerMap)
	
	// Create a dummy remote manager that just returns the mock provider
	rm := config.NewRemoteManager("", 0)
	
	// Inject fake strategy so router picks mock
	// Note: We need a slight hack since the defaultEngine looks for local_vllm or openai.
	// But let's cheat and register mock as "openai" for testing purposes.
	providerMap["openai"] = providerMap["mock"]

	srv := server.NewServer(rm, engine)
	addr := ":8081"
	fmt.Printf("[Mock] Starting Mock Gateway on %s\n", addr)
	if err := srv.Start(addr); err != nil {
		logger.Fatalf("Mock Server stopped: %v", err)
	}
}
