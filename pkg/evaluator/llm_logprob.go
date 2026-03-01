package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/logger"
)

// LLMLogprobEvaluator uses log probabilities of specific tokens to return a continuous decimal score (float 0.0~1.0).
// When using Ollama protocol (which does not support logprobs), it falls back to content-based parsing.
type LLMLogprobEvaluator struct {
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

func NewLLMLogprobEvaluator(cfg config.EvaluatorConfig) (*LLMLogprobEvaluator, error) {
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

	return &LLMLogprobEvaluator{
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

func (e *LLMLogprobEvaluator) Name() string {
	return e.name
}

func (e *LLMLogprobEvaluator) HistoryRounds() int {
	return e.historyRounds
}

func (e *LLMLogprobEvaluator) Evaluate(ctx context.Context, messages []models.Message) (*EvaluationResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	currentMsg := messages[len(messages)-1]

	var historyBuilder strings.Builder
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

	if e.protocol == "ollama" {
		return e.evaluateOllama(ctx, promptBuf.String())
	}
	return e.evaluateOpenAI(ctx, promptBuf.String())
}

// evaluateOllama uses Ollama native /api/chat with think=false, and falls back to content-based scoring
// since Ollama does not support logprobs.
func (e *LLMLogprobEvaluator) evaluateOllama(ctx context.Context, prompt string) (*EvaluationResult, error) {
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

	content, err := parseOllamaContent(bodyBytes)
	if err != nil {
		return nil, err
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty content in Ollama response")
	}

	// Fallback: parse content for "0" or "1" as discrete score
	score, err := strconv.ParseFloat(content[:1], 64)
	if err != nil {
		// Try to find the first digit
		for _, c := range content {
			if c == '0' || c == '1' {
				score = float64(c - '0')
				err = nil
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("no '0' or '1' found in Ollama content: %q", content)
		}
	}

	return &EvaluationResult{
		Dimension: e.name,
		Score:     score,
	}, nil
}

// evaluateOpenAI uses the OpenAI-compatible /v1/chat/completions endpoint with logprobs
func (e *LLMLogprobEvaluator) evaluateOpenAI(ctx context.Context, prompt string) (*EvaluationResult, error) {
	reqBody := map[string]interface{}{
		"model": e.model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature":      0.0,
		"max_tokens":       150, // Allow enough tokens for reasoning models
		"logprobs":         true,
		"top_logprobs":     2,
		"disable_thinking": true,
		"think":            false,
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

	type TopLogprob struct {
		Token   string  `json:"token"`
		Logprob float64 `json:"logprob"`
	}

	var openAIResp struct {
		Choices []struct {
			Logprobs struct {
				Content []struct {
					Token       string       `json:"token"`
					TopLogprobs []TopLogprob `json:"top_logprobs"`
				} `json:"content"`
			} `json:"logprobs"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(openAIResp.Choices) == 0 || len(openAIResp.Choices[0].Logprobs.Content) == 0 {
		return nil, fmt.Errorf("missing logprobs in response")
	}

	tokens := openAIResp.Choices[0].Logprobs.Content
	var topLogprobs []TopLogprob

	// Search backwards for the actual '0' or '1' output token
	for i := len(tokens) - 1; i >= 0; i-- {
		tk := strings.TrimSpace(tokens[i].Token)
		if tk == "0" || tk == "1" {
			topLogprobs = tokens[i].TopLogprobs
			break
		}
	}

	if len(topLogprobs) == 0 {
		return nil, fmt.Errorf("missing '0' or '1' token in logprobs context")
	}

	val0 := math.Inf(-1)
	val1 := math.Inf(-1)

	for _, tlp := range topLogprobs {
		tk := strings.TrimSpace(tlp.Token)
		if tk == "0" {
			val0 = tlp.Logprob
		} else if tk == "1" {
			val1 = tlp.Logprob
		}
	}

	// Softmax calculation
	score := 0.0
	if val0 == math.Inf(-1) && val1 == math.Inf(-1) {
		return nil, fmt.Errorf("neither '0' nor '1' found in top logprobs")
	} else if val0 == math.Inf(-1) {
		score = 1.0
	} else if val1 == math.Inf(-1) {
		score = 0.0
	} else {
		maxVal := val0
		if val1 > val0 {
			maxVal = val1
		}
		exp0 := math.Exp(val0 - maxVal)
		exp1 := math.Exp(val1 - maxVal)
		score = exp1 / (exp0 + exp1)
	}

	return &EvaluationResult{
		Dimension: e.name,
		Score:     score,
	}, nil
}
