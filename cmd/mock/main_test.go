package main

import (
	"context"
	"testing"

	"agentic-llm-gateway/internal/models"
)

func TestMockProvider_Name(t *testing.T) {
	mp := &MockProvider{}
	if mp.Name() != "mock" {
		t.Errorf("expected mock, got %q", mp.Name())
	}
}

func TestMockProvider_ChatCompletion(t *testing.T) {
	mp := &MockProvider{}
	req := &models.ChatCompletionRequest{
		Model: "test",
	}
	resp, err := mp.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test" {
		t.Errorf("expected test, got %q", resp.Model)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "Hello! this is a mock non-streaming response." {
		t.Errorf("unexpected content: %v", resp.Choices)
	}
}

func TestMockProvider_ChatCompletionStream(t *testing.T) {
	mp := &MockProvider{}
	req := &models.ChatCompletionRequest{
		Model: "test",
	}
	ch := make(chan *models.ChatCompletionStreamResponse, 20)

	err := mp.ChatCompletionStream(context.Background(), req, ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := 0
	for resp := range ch {
		if resp.Model != "test" {
			t.Errorf("expected test model, got %q", resp.Model)
		}
		count++
	}

	if count != 11 {
		t.Errorf("expected 11 chunks, got %d", count)
	}
}
