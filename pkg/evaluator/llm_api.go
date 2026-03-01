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

	timeout := 100 * time.Millisecond
	if cfg.TimeoutMs > 0 {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

	return &LLMAPIEvaluator{
		name:          cfg.Name,
		endpoint:      cfg.Endpoint,
		model:         cfg.Model,
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

	// Prepare OpenAI request body
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": promptBuf.String(),
			},
		},
		"temperature":      0.0,
		"max_tokens":       1, // We only need 1 token (0 or 1)
		"disable_thinking": true,
	}
	if len(e.logitBias) > 0 {
		reqBody["logit_bias"] = e.logitBias
	}

	reqBytes, err := json.Marshal(reqBody)
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

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in response")
	}

	content := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	score, err := strconv.ParseFloat(content, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse score from content %q: %w", content, err)
	}

	return &EvaluationResult{
		Dimension: e.name,
		Score:     score,
	}, nil
}
