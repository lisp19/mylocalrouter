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
	"time"

	"localrouter/internal/models"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

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
	return "google"
}

// Google Gemini API Structures
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
			role = "user" // simplificiation for older gemini apis which didn't have system instructions
		} else {
			role = "user"
		}

		greq.Contents = append(greq.Contents, geminiContent{
			Role: role,
			Parts: []geminiPart{
				{Text: m.Content},
			},
		})
	}
	return greq
}

func (p *Provider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	greq := mapRequest(req)
	data, err := json.Marshal(greq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)
	hreq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google api error %d: %s", resp.StatusCode, string(body))
	}

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
		Model:   req.Model,
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
	}, nil
}

func (p *Provider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error {
	greq := mapRequest(req)
	data, err := json.Marshal(greq)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, req.Model, p.apiKey)
	hreq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(hreq)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("google api error %d: %s", resp.StatusCode, string(body))
	}

	go func() {
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

			if bytes.HasPrefix(line, []byte("data: ")) {
				payload := bytes.TrimPrefix(line, []byte("data: "))
				if bytes.Equal(payload, []byte("[DONE]")) {
					break
				}
				
				var gresp geminiResponse
				if err := json.Unmarshal(payload, &gresp); err != nil {
					continue
				}

				if len(gresp.Candidates) > 0 && len(gresp.Candidates[0].Content.Parts) > 0 {
					var chunk models.ChatCompletionStreamResponse
					chunk.ID = fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano())
					chunk.Object = "chat.completion.chunk"
					chunk.Created = time.Now().Unix()
					chunk.Model = req.Model
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
					case streamChan <- &chunk:
					}
				}
			}
		}

		if err := scanner.Err(); err != nil && err != context.Canceled {
			fmt.Printf("[Google] Stream error: %v\n", err)
		}
	}()

	return nil
}
