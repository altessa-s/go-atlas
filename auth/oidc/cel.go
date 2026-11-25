// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/runtime/panics"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	oidcrules "github.com/altessa-s/go-atlas/auth/oidc/rules"
)

const (
	// DefaultCELCacheSize is the maximum number of compiled CEL programs to cache.
	DefaultCELCacheSize = oidcrules.DefaultCELCacheSize

	// DefaultCELCostLimit is the maximum runtime cost for a single CEL evaluation.
	// cel-go charges ~1 cost unit per operation; 10000 is generous for claim checks.
	DefaultCELCostLimit = oidcrules.DefaultCELCostLimit

	// DefaultCELMaxExpressionLength is the maximum allowed CEL expression length in bytes.
	DefaultCELMaxExpressionLength = oidcrules.DefaultCELMaxExpressionLength
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
	name    string            // Rule name for error messages
	message string            // Custom error message (optional)
	matcher PresetMatcherFunc // Pre-compiled CEL matcher
}

// CELMatcher creates a matcher using a CEL expression.
// The expression must return boolean and has access to 'claims' variable.
// Invalid expressions return a matcher that always returns false.
//
// Example:
//
//	matcher := oidc.CELMatcher(ctx, "claims.role == 'admin' && has(claims.email)")
func CELMatcher(ctx context.Context, expression string) PresetMatcherFunc {
	matcher, err := oidcrules.CachedCEL(ctx, expression)
	if err != nil {
		slog.Default().Error("oidc: CEL preset matcher failed to compile; it will never match", "expression", expression, "error", err)
		return func(map[string]any) bool { return false }
	}
	return matcher
}

// MustCELMatcher creates a CEL matcher, panicking if the expression is invalid.
// Use this for compile-time expressions to catch errors at startup.
//
// Example:
//
//	var AdminMatcher = oidc.MustCELMatcher("claims.role == 'admin'")
func MustCELMatcher(expression string) PresetMatcherFunc {
	return panics.MustResult(oidcrules.CachedCEL(context.Background(), expression))
}

// validateCELRules prepares the default policy's matchers.
func (p *policyEvaluator) validateCELRules() error {
	if p.verifierOptions == nil || len(p.verifierOptions.celRules) == 0 {
		return nil
	}

	compiled, err := compileCELRules(p.verifierOptions.celRules)
	if err != nil {
		return err
	}
	p.celCompiledRules = compiled
	// The source rules stay on p.verifierOptions: per-call options clone the
	// defaults, so clearing them here would silently drop the default CEL
	// policy from every ValidateTokenWithOptions call that passes options.
	return nil
}

// compileVerifierCELRules compiles CEL rules from verifierOptions, returning compiled rules.
// The celRules field is cleared after compilation.
func compileVerifierCELRules(ops *verifierOptions) ([]celPreCompiledValidationRule, error) {
	if ops == nil || len(ops.celRules) == 0 {
		return nil, nil
	}

	compiled, err := compileCELRules(ops.celRules)
	if err != nil {
		return nil, err
	}
	ops.celRules = nil // Clear source rules after compilation
	return compiled, nil
}

// compileCELRules compiles CEL rules into pre-compiled matchers. A malformed
// rule (empty name/expression or an expression that fails to compile) is a
// configuration error and is returned, not silently skipped — mirroring the
// strict checks in the service-config loader so a typo cannot quietly drop a
// validation rule (which would weaken validation).
func compileCELRules(rules []CELValidationRule) ([]celPreCompiledValidationRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	validRules := make([]celPreCompiledValidationRule, 0, len(rules))
	for i, rule := range rules {
		if rule.Name == "" {
			return nil, coreerrs.Wrapf(oidcrules.ErrInvalidCEL, "cel_rules[%d].name is required", i)
		}
		if rule.Expression == "" {
			return nil, coreerrs.Wrapf(oidcrules.ErrInvalidCEL, "cel_rules[%q].expression is required", rule.Name)
		}

		// Compile CEL expression, reusing a cached program: per-call options
		// recompile the default rules on every call.
		matcher, err := oidcrules.CompileCEL(rule.Expression)
		if err != nil {
			return nil, coreerrs.Wrapf(oidcrules.ErrInvalidCEL, "cel_rules[%q]: %v", rule.Name, err)
		}

		validRules = append(validRules, celPreCompiledValidationRule{name: rule.Name, message: rule.Message, matcher: matcher})
	}
	return validRules, nil
}
