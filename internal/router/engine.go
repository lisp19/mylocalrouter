package router

import (
	"context"
	"fmt"
	"log"

	"localrouter/internal/config"
	"localrouter/internal/models"
	"localrouter/internal/providers"

	"localrouter/pkg/evaluator"

	"github.com/expr-lang/expr"
)

// StrategyEngine directs a ChatCompletionRequest to the correct Provider based on the RemoteStrategy.
type StrategyEngine interface {
	SelectProvider(req *models.ChatCompletionRequest, remoteCfg *config.RemoteStrategy) (providers.Provider, string, error)
}

type defaultEngine struct {
	providerMap map[string]providers.Provider
	evaluators  []evaluator.Evaluator
}

// NewEngine initializes a routing expression engine.
func NewEngine(pMap map[string]providers.Provider) StrategyEngine {
	var evals []evaluator.Evaluator
	if config.GlobalConfig != nil && config.GlobalConfig.GenerativeRouting != nil && config.GlobalConfig.GenerativeRouting.Enabled {
		for _, eCfg := range config.GlobalConfig.GenerativeRouting.Evaluators {
			if eCfg.Type == "llm_api" {
				if ev, err := evaluator.NewLLMAPIEvaluator(eCfg); err == nil {
					evals = append(evals, ev)
				} else {
					log.Printf("[Router] Failed to init LLM API Evaluator %s: %v", eCfg.Name, err)
				}
			} else if eCfg.Type == "builtin" {
				evals = append(evals, evaluator.NewBuiltinLengthEvaluator(eCfg))
			}
		}
	}
	return &defaultEngine{
		providerMap: pMap,
		evaluators:  evals,
	}
}

// Env is the environment passed into the expression engine
type Env struct {
	Req *models.ChatCompletionRequest
	Cfg *config.RemoteStrategy
}

func (e *defaultEngine) SelectProvider(req *models.ChatCompletionRequest, remoteCfg *config.RemoteStrategy) (providers.Provider, string, error) {
	// Generative Smart Routing
	if config.GlobalConfig != nil && config.GlobalConfig.GenerativeRouting != nil && config.GlobalConfig.GenerativeRouting.Enabled && len(e.evaluators) > 0 {
		ctx := context.Background() // A real implementation would pass request context
		genCfg := config.GlobalConfig.GenerativeRouting
		vectors := evaluator.EvaluateAll(ctx, req.Messages, genCfg.GlobalTimeoutMs, e.evaluators)

		// Temporary strict local first logic until Stage 5 Resolver is implemented
		targetProvider := ""
		if len(vectors) == len(e.evaluators) { // vectors are complete
			allZero := true
			for _, score := range vectors {
				if score > 0 {
					allZero = false
					break
				}
			}
			if allZero {
				targetProvider = "local_vllm" // Example local provider
			}
		}

		if targetProvider == "" {
			targetProvider = genCfg.FallbackProvider
		}

		if targetProvider != "" {
			if p, ok := e.providerMap[targetProvider]; ok {
				return p, req.Model, nil
			}
			log.Printf("[Router] Generative Routing fallback provider %s not found, continuing to normal routing...", targetProvider)
		}
	}

	// Fallback if no strategy defined
	if remoteCfg == nil || remoteCfg.Strategy == "" {
		log.Printf("[Router] No remote strategy defined, defaulting to google")
		if p, ok := e.providerMap["google"]; ok {
			return p, req.Model, nil
		}
		if p, ok := e.providerMap["local_vllm"]; ok {
			return p, req.Model, nil
		}
		return nil, "", fmt.Errorf("no strategy and no sensible default providers found")
	}

	// Example usage of expr: evaluate custom routing condition
	// For example, if we wanted to dynamically route based on the request's length using expr, we could do:
	// program, _ := expr.Compile("Cfg.Strategy == 'remote' && len(Req.Messages) > 0", expr.Env(Env{}))
	// res, _ := expr.Run(program, Env{Req: req, Cfg: remoteCfg})
	// log.Printf("[Router] Expr Result: %v", res)

	if config.GlobalConfig != nil && config.GlobalConfig.RemoteStrategy.Expression != "" {
		program, err := expr.Compile(config.GlobalConfig.RemoteStrategy.Expression, expr.Env(Env{}))
		if err == nil {
			res, err := expr.Run(program, Env{Req: req, Cfg: remoteCfg})
			if err == nil {
				if providerName, ok := res.(string); ok {
					if p, exists := e.providerMap[providerName]; exists {
						targetModel := req.Model

						// If the Expr evaluated provider matches what this provider is mapped to in Remote strategy
						// Check if it's the generic remote provider or local vllm
						if providerName == "local_vllm" && remoteCfg.LocalModel != "" {
							targetModel = remoteCfg.LocalModel
						} else if remoteCfg.RemoteModel != "" {
							// If it's a cloud provider, default to the remote model.
							targetModel = remoteCfg.RemoteModel
						}

						return p, targetModel, nil
					}
					log.Printf("[Router] Expr matched unknown provider: %v", providerName)
				}
			} else {
				log.Printf("[Router] Expr Run Error: %v", err)
			}
		} else {
			log.Printf("[Router] Expr Compile Error: %v", err)
		}
	}

	// Since local_router.md says "based on remote JSON return ... local or remote", we evaluate strictly:
	if remoteCfg.Strategy == "remote" {
		targetProvider := remoteCfg.RemoteProvider
		if targetProvider == "" {
			targetProvider = "google" // user requested default fallback to google
		}

		p, ok := e.providerMap[targetProvider]
		if !ok {
			return nil, "", fmt.Errorf("remote provider '%s' not configured", targetProvider)
		}
		return p, remoteCfg.RemoteModel, nil
	}

	if remoteCfg.Strategy == "local" {
		p, ok := e.providerMap["local_vllm"]
		if !ok {
			return nil, "", fmt.Errorf("local provider 'local_vllm' not configured")
		}
		return p, remoteCfg.LocalModel, nil
	}

	return nil, "", fmt.Errorf("unknown strategy: %s", remoteCfg.Strategy)
}
