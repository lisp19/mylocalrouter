package router

import (
	"fmt"
	"log"

	"localrouter/internal/config"
	"localrouter/internal/models"
	"localrouter/internal/providers"

	"github.com/expr-lang/expr"
)

// StrategyEngine directs a ChatCompletionRequest to the correct Provider based on the RemoteStrategy.
type StrategyEngine interface {
	SelectProvider(req *models.ChatCompletionRequest, remoteCfg *config.RemoteStrategy) (providers.Provider, string, error)
}

type defaultEngine struct {
	providerMap map[string]providers.Provider
}

// NewEngine initializes a routing expression engine.
func NewEngine(pMap map[string]providers.Provider) StrategyEngine {
	return &defaultEngine{providerMap: pMap}
}

// Env is the environment passed into the expression engine
type Env struct {
	Req *models.ChatCompletionRequest
	Cfg *config.RemoteStrategy
}

func (e *defaultEngine) SelectProvider(req *models.ChatCompletionRequest, remoteCfg *config.RemoteStrategy) (providers.Provider, string, error) {
	// Fallback if no strategy defined
	if remoteCfg == nil || remoteCfg.Strategy == "" {
		log.Printf("[Router] No remote strategy defined, defaulting to local_vllm")
		if p, ok := e.providerMap["local_vllm"]; ok {
			return p, req.Model, nil
		}
		if p, ok := e.providerMap["openai"]; ok {
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
						if providerName == "openai" && remoteCfg != nil && remoteCfg.RemoteModel != "" {
							targetModel = remoteCfg.RemoteModel
						} else if providerName == "local_vllm" && remoteCfg != nil && remoteCfg.LocalModel != "" {
							targetModel = remoteCfg.LocalModel
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
		// Just forward to standard remote cloud provider via OpenAI adapter
		p, ok := e.providerMap["openai"]
		if !ok {
			return nil, "", fmt.Errorf("remote provider 'openai' not configured")
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
