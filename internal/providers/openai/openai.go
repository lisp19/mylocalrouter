package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"localrouter/internal/models"
	"localrouter/pkg/httputil"
)

// Provider implements the OpenAI compatible upstream provider.
type Provider struct {
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewProvider creates a new generic OpenAI provider instance.
func NewProvider(name, apiKey, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Provider{
		name:    name,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string {
	return p.name
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

	return p.client.Do(req)
}

func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Stream = false
	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	var completion models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, err
	}

	return &completion, nil
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	req.Stream = true
	resp, err := p.postRequest(ctx, "/chat/completions", req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	go func() {
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
			fmt.Printf("[%s] Stream error: %v\n", p.name, err)
		}
	}()

	return nil
}
