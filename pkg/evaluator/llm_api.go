package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/logger"
)

// LLMAPIEvaluator evaluates using a standard OpenAI Chat API compatible endpoint
type LLMAPIEvaluator struct {
	name          string
	endpoint      string
	model         string
	protocol      string // "ollama" or "openai"
	historyRounds int
	timeoutMs     int
	logitBias     map[string]int
	promptTpl     *template.Template
	client        *http.Client
}

type tmplData struct {
	History string
	Current string
}

func NewLLMAPIEvaluator(cfg config.EvaluatorConfig) (*LLMAPIEvaluator, error) {
	tpl, err := template.New(cfg.Name).Parse(cfg.PromptTemplate)
	if err != nil {
		return nil, fmt.Errorf("invalid prompt template: %w", err)
	}

	timeout := 10 * time.Second
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

	protocol := cfg.Protocol
	if protocol == "" {
		protocol = "ollama"
	}

	return &LLMAPIEvaluator{
		name:          cfg.Name,
		endpoint:      cfg.Endpoint,
		model:         cfg.Model,
		protocol:      protocol,
		historyRounds: cfg.HistoryRounds,
		timeoutMs:     cfg.TimeoutMs,
		logitBias:     cfg.LogitBias,
		promptTpl:     tpl,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (e *LLMAPIEvaluator) Name() string {
	return e.name
}

func (e *LLMAPIEvaluator) HistoryRounds() int {
	return e.historyRounds
}

func (e *LLMAPIEvaluator) Evaluate(ctx context.Context, messages []models.Message) (*EvaluationResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Prepare data for template
	currentMsg := messages[len(messages)-1]

	var historyBuilder strings.Builder
	// Get up to historyRounds context
	startIdx := len(messages) - 1 - e.historyRounds
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < len(messages)-1; i++ {
		historyBuilder.WriteString(fmt.Sprintf("%s: %s\n", messages[i].Role, messages[i].Content))
	}

	data := tmplData{
		History: historyBuilder.String(),
		Current: currentMsg.Content,
	}

	var promptBuf bytes.Buffer
	if err := e.promptTpl.Execute(&promptBuf, data); err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var reqBytes []byte
	var err error

	if e.protocol == "ollama" {
		reqBytes, err = e.buildOllamaRequest(promptBuf.String())
	} else {
		reqBytes, err = e.buildOpenAIRequest(promptBuf.String())
	}
	if err != nil {
		return nil, err
	}

	logger.Printf("[Evaluator %s] Verbose Input: %s", e.name, string(reqBytes))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	logger.Printf("[Evaluator %s] Verbose Output: %s", e.name, string(bodyBytes))

	var content string
	if e.protocol == "ollama" {
		content, err = parseOllamaContent(bodyBytes)
	} else {
		content, err = parseOpenAIContent(bodyBytes)
	}
	if err != nil {
		return nil, err
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty content in response")
	}

	// Extract the first digit from the content
	var scoreStr string
	for _, c := range content {
		if c >= '0' && c <= '9' {
			scoreStr += string(c)
			break
		}
	}
	if len(scoreStr) == 0 {
		return nil, fmt.Errorf("no numeric score found in content: %q", content)
	}

	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse score from content %q: %w", content, err)
	}

	return &EvaluationResult{
		Dimension: e.name,
		Score:     score,
	}, nil
}

// buildOllamaRequest constructs a request body for the Ollama /api/chat endpoint
func (e *LLMAPIEvaluator) buildOllamaRequest(prompt string) ([]byte, error) {
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"stream": false,
		"think":  false, // Disable reasoning for Ollama thinking models
		"options": map[string]interface{}{
			"temperature": 0.0,
			"num_predict": 1,
		},
	}
	return json.Marshal(reqBody)
}

// buildOpenAIRequest constructs a request body for the OpenAI /v1/chat/completions endpoint
func (e *LLMAPIEvaluator) buildOpenAIRequest(prompt string) ([]byte, error) {
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature":      0.0,
		"max_tokens":       150, // Allow enough tokens for models with reasoning
		"disable_thinking": true,
		"think":            false,
	}
	if len(e.logitBias) > 0 {
		reqBody["logit_bias"] = e.logitBias
	}
	return json.Marshal(reqBody)
}

// parseOllamaContent extracts the response content from an Ollama /api/chat response
func parseOllamaContent(body []byte) (string, error) {
	var ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("failed to decode Ollama response: %w", err)
	}
	return ollamaResp.Message.Content, nil
}

// parseOpenAIContent extracts the response content from an OpenAI /v1/chat/completions response
func parseOpenAIContent(body []byte) (string, error) {
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("failed to decode OpenAI response: %w", err)
	}
	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices in response")
	}
	return openAIResp.Choices[0].Message.Content, nil
}
