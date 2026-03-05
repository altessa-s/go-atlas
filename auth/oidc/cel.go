// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/cache/lru"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// getCELEnvironment returns the shared CEL environment, initialized on first call.
var getCELEnvironment = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("claims", cel.MapType(cel.StringType, cel.DynType)),
	)
})

// getCELCache returns the singleton CEL program cache, initialized on first call.
var getCELCache = sync.OnceValue(func() lru.Cacher[string, cel.Program] {
	cache, err := lru.NewCache[string, cel.Program](DefaultCELCacheSize)
	if err != nil {
		slog.Error("failed to create CEL cache, CEL matchers will recompile on every use",
			"error", err,
			"cache_size", DefaultCELCacheSize)
		return nil
	}
	return cache
})

const (
	// DefaultCELCacheSize is the maximum number of compiled CEL programs to cache.
	DefaultCELCacheSize = 1000

	// DefaultCELCostLimit is the maximum runtime cost for a single CEL evaluation.
	// cel-go charges ~1 cost unit per operation; 10000 is generous for claim checks.
	DefaultCELCostLimit uint64 = 10_000

	// DefaultCELMaxExpressionLength is the maximum allowed CEL expression length in bytes.
	DefaultCELMaxExpressionLength = 4096
)

// CELValidationRule represents a single CEL-based validation rule.
// The Expression must return a boolean and has access to the 'claims' variable.
//
// Example:
//
//	rule := CELValidationRule{Name: "admin-only", Expression: "claims.role == 'admin'"}
type CELValidationRule struct {
	// Name is the rule identifier used in error messages.
	Name string

	// Expression is the CEL expression to evaluate (must return boolean).
	Expression string

	// Message is a custom error message to use when the rule fails (optional).
	Message string
}

// celPreCompiledValidationRule is the internal representation with pre-compiled matcher.
type celPreCompiledValidationRule struct {
	name       string            // Rule name for error messages
	expression string            // CEL expression to evaluate
	message    string            // Custom error message (optional)
	matcher    PresetMatcherFunc // Pre-compiled CEL matcher
}

// CELMatcher creates a matcher using a CEL expression.
// The expression must return boolean and has access to 'claims' variable.
// Invalid expressions return a matcher that always returns false.
//
// Example:
//
//	matcher := oidc.CELMatcher(ctx, "claims.role == 'admin' && has(claims.email)")
func CELMatcher(ctx context.Context, expression string) PresetMatcherFunc {
	cache := getCELCache()
	if cache == nil {
		program, err := compileCELExpression(expression)
		if err != nil {
			return func(claims map[string]any) bool { return false }
		}
		return func(claims map[string]any) bool {
			return evaluateCELProgram(program, claims)
		}
	}

	program, err := cache.GetOrCompute(ctx, expression, func(ctx context.Context) (cel.Program, error) {
		return compileCELExpression(expression)
	})

	if err != nil {
		return func(claims map[string]any) bool {
			return false
		}
	}

	// Return matcher that directly uses the pre-compiled program
	return func(claims map[string]any) bool {
		return evaluateCELProgram(program, claims)
	}
}

// MustCELMatcher creates a CEL matcher, panicking if the expression is invalid.
// Use this for compile-time expressions to catch errors at startup.
//
// Example:
//
//	var AdminMatcher = oidc.MustCELMatcher("claims.role == 'admin'")
func MustCELMatcher(expression string) PresetMatcherFunc {
	cache := getCELCache()
	if cache == nil {
		program := panics.MustResult(compileCELExpression(expression))
		return func(claims map[string]any) bool {
			return evaluateCELProgram(program, claims)
		}
	}

	program := panics.MustResult(cache.GetOrCompute(context.Background(), expression, func(ctx context.Context) (cel.Program, error) {
		return compileCELExpression(expression)
	}))

	// Return matcher directly to avoid double compilation
	return func(claims map[string]any) bool {
		return evaluateCELProgram(program, claims)
	}
}

// validateCELRules compiles CEL rules from verifier options and stores them in the Provider.
func (p *Provider) validateCELRules() {
	if p.verifierOptions == nil || len(p.verifierOptions.celRules) == 0 {
		return
	}

	p.celCompiledRules = compileCELRules(p.verifierOptions.celRules, p.logger)
	p.verifierOptions.celRules = nil // Clear source rules after compilation
}

// compileVerifierCELRules compiles CEL rules from verifierOptions, returning compiled rules.
// The celRules field is cleared after compilation.
func compileVerifierCELRules(ops *verifierOptions, logger *slog.Logger) []celPreCompiledValidationRule {
	if ops == nil || len(ops.celRules) == 0 {
		return nil
	}

	compiled := compileCELRules(ops.celRules, logger)
	ops.celRules = nil // Clear source rules after compilation
	return compiled
}

// compileCELRules compiles CEL rules into pre-compiled matchers, filtering invalid ones.
func compileCELRules(rules []CELValidationRule, logger *slog.Logger) []celPreCompiledValidationRule {
	if len(rules) == 0 {
		return nil
	}

	validRules := make([]celPreCompiledValidationRule, 0, len(rules))
	for _, rule := range rules {
		// Validate rule name and expression
		if rule.Name == "" || rule.Expression == "" {
			logger.Warn("skipping invalid CEL rule", "name", rule.Name, "has_expression", rule.Expression != "")
			continue
		}

		// Compile CEL expression
		program, err := compileCELExpression(rule.Expression)
		if err != nil {
			logger.Warn("skipping invalid CEL rule", "name", rule.Name, slog.Any("error", err))
			continue
		}

		// Prime cache to avoid cold-start compilation
		if cache := getCELCache(); cache != nil {
			cache.Put(rule.Expression, program)
		}

		// Capture program in local variable to avoid closure capture bug
		prog := program
		validRules = append(validRules, celPreCompiledValidationRule{
			name:       rule.Name,
			expression: rule.Expression,
			message:    rule.Message,
			matcher: func(claims map[string]any) bool {
				return evaluateCELProgram(prog, claims)
			},
		})
	}
	return validRules
}

// compileCELExpression compiles a CEL expression into a program.
// Returns error if expression is invalid or does not return boolean.
func compileCELExpression(expression string) (cel.Program, error) {
	if len(expression) > DefaultCELMaxExpressionLength {
		return nil, fmt.Errorf("expression too long (%d bytes, max %d)", len(expression), DefaultCELMaxExpressionLength)
	}

	env, err := getCELEnvironment()
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, coreerrs.WrapOperation(issues.Err(), "compile expression")
	}

	// Check that the expression returns a boolean
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("expression must return boolean, got %v", ast.OutputType())
	}

	program, err := env.Program(ast, cel.CostLimit(DefaultCELCostLimit))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create program")
	}

	return program, nil
}

// evaluateCELProgram evaluates a compiled CEL program with claims, returning false on error.
func evaluateCELProgram(program cel.Program, claims map[string]any) bool {
	result, _, err := program.Eval(map[string]any{
		"claims": claims,
	})
	if err != nil {
		return false
	}

	boolResult, ok := result.(types.Bool)
	if !ok {
		return false
	}

	return bool(boolResult)
}
