package strategy

import (
	"log"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"localrouter/internal/config"
)

// ExpressionResolver dynamically parses logical expressions
type ExpressionResolver struct {
	rules           []CompiledRule
	defaultProvider string
}

// CompiledRule caches the AST or byte code of the parsed condition
type CompiledRule struct {
	Program        *vm.Program
	TargetProvider string
}

func NewExpressionResolver(cfg config.ResolutionStrategyConfig) *ExpressionResolver {
	var compiledRules []CompiledRule

	for _, rule := range cfg.Rules {
		// Compile expression once at startup
		program, err := expr.Compile(rule.Condition, expr.AllowUndefinedVariables())
		if err != nil {
			log.Printf("[Strategy] Failed to compile expression %q: %v", rule.Condition, err)
			continue
		}

		compiledRules = append(compiledRules, CompiledRule{
			Program:        program,
			TargetProvider: rule.TargetProvider,
		})
	}

	return &ExpressionResolver{
		rules:           compiledRules,
		defaultProvider: cfg.DefaultProvider,
	}
}

func (e *ExpressionResolver) Name() string {
	return "dynamic_expression"
}

func (e *ExpressionResolver) Resolve(vector map[string]float64) string {
	env := make(map[string]interface{}, len(vector))
	for k, v := range vector {
		env[k] = v
	}

	for _, rule := range e.rules {
		matched, err := expr.Run(rule.Program, env)
		if err == nil {
			if b, ok := matched.(bool); ok && b {
				return rule.TargetProvider
			}
		}
		// If evaluation fails (e.g. unknown variable timeout), we just log and fall through to next rule
	}

	return e.defaultProvider
}
