package strategy

import "localrouter/internal/config"

// Resolver defines the unified interface for intent vector resolution strategies
type Resolver interface {
	// Name returns the unique identifier for the strategy
	Name() string
	// Resolve takes an intent vector and returns the Target Provider Name.
	// Returns an empty string if it cannot resolve, yielding to the fallback.
	Resolve(vector map[string]float64) string
}

// NewResolver initializes a resolver based on the configuration
func NewResolver(cfg config.ResolutionStrategyConfig) Resolver {
	switch cfg.Type {
	case "dynamic_expression":
		return NewExpressionResolver(cfg)
	case "strict_local_first":
		return NewStrictLocalResolver(cfg)
	default:
		return nil
	}
}
