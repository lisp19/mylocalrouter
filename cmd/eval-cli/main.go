package main

import (
	"agentic-llm-gateway/pkg/logger"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"agentic-llm-gateway/internal/config"
	"agentic-llm-gateway/internal/models"
	"agentic-llm-gateway/pkg/evaluator"
)

func main() {
	var configPath string
	var evaluatorName string
	var inputPath string

	flag.StringVar(&configPath, "config", "config.yaml", "Path to config file")
	flag.StringVar(&evaluatorName, "evaluator", "", "Name of the evaluator to run")
	flag.StringVar(&inputPath, "input", "mock_chat.json", "Path to mock chat history JSON")
	flag.Parse()

	if evaluatorName == "" {
		logger.Fatal("Please specify an evaluator using --evaluator")
	}

	// 1. Load config manually to skip full system defaults
	f, err := os.Open(configPath)
	if err != nil {
		logger.Fatalf("Failed to open config %s: %v", configPath, err)
	}
	defer f.Close()

	var conf config.Config
	if err := yaml.NewDecoder(f).Decode(&conf); err != nil {
		logger.Fatalf("Failed to decode config: %v", err)
	}

	if conf.GenerativeRouting == nil {
		logger.Fatal("Config does not have a generative_routing section")
	}

	// 2. Find Evaluator
	var evalCfg config.EvaluatorConfig
	found := false
	for _, e := range conf.GenerativeRouting.Evaluators {
		if e.Name == evaluatorName {
			evalCfg = e
			found = true
			break
		}
	}
	if !found {
		logger.Fatalf("Evaluator %s not found in config", evaluatorName)
	}

	// 3. Initialize Evaluator
	var ev evaluator.Evaluator
	if evalCfg.Type == "llm_api" {
		ev, err = evaluator.NewLLMAPIEvaluator(evalCfg)
	} else if evalCfg.Type == "builtin" {
		ev = evaluator.NewBuiltinLengthEvaluator(evalCfg)
	} else {
		logger.Fatalf("Unknown evaluator type: %s", evalCfg.Type)
	}
	if err != nil {
		logger.Fatalf("Failed to init evaluator: %v", err)
	}

	// 4. Load Mock Chat
	chatData, err := os.ReadFile(inputPath)
	if err != nil {
		logger.Fatalf("Failed to read input %s: %v", inputPath, err)
	}

	var req models.ChatCompletionRequest
	if err := json.Unmarshal(chatData, &req); err != nil {
		logger.Fatalf("Failed to parse mock chat: %v", err)
	}

	// 5. Execute Eval
	ctx := context.Background()

	start := time.Now()
	res, err := ev.Evaluate(ctx, req.Messages)
	elapsed := time.Since(start)

	if err != nil {
		logger.Fatalf("Evaluation failed: %v", err)
	}

	// 6. Output Result
	fmt.Println("=== Evaluation Result ===")
	fmt.Printf("Evaluator Dimension: %s\n", res.Dimension)
	fmt.Printf("Score:               %v\n", res.Score)
	fmt.Printf("Time Taken (TTFT):   %s\n", elapsed)
}
