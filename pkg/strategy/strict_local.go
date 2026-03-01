package strategy

import "agentic-llm-gateway/internal/config"

// StrictLocalResolver routes to local if complexity and context_rel are 0, else remote.
// This is a static hardcoded fallback strategy.
type StrictLocalResolver struct {
	defaultProvider string
}

func NewStrictLocalResolver(cfg config.ResolutionStrategyConfig) *StrictLocalResolver {
	return &StrictLocalResolver{
		defaultProvider: cfg.DefaultProvider,
	}
}

func (s *StrictLocalResolver) Name() string {
	return "strict_local_first"
}

func (s *StrictLocalResolver) Resolve(vector map[string]float64) string {
	target := "local_vllm" // hardcoded for this simple built-in rule based on spec

	comp, okC := vector["complexity"]
	ctxRel, okR := vector["context_rel"]
	lenCheck, okL := vector["length_check"]

	if !okC || !okR || !okL {
		return s.defaultProvider // Incomplete vector
	}

	if comp == 0.0 && ctxRel == 0.0 && lenCheck == 0.0 {
		return target
	}
	return s.defaultProvider
}
