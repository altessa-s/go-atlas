// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"slices"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ValidationPreset represents a named, reusable set of validation options.
type ValidationPreset struct {
	name             string
	options          []ValidationOption
	compiledVerifier *verifierOptions               // Pre-compiled verifier options
	compiledCELRules []celPreCompiledValidationRule // Pre-compiled CEL rules
}

// NewValidationPreset creates a named validation preset with the given options.
//
// Example:
//
//	preset := oidc.NewValidationPreset("strict", oidc.WithValidationRequiredClaims("sub", "email"))
func NewValidationPreset(name string, opts ...ValidationOption) *ValidationPreset {
	return &ValidationPreset{
		name:    name,
		options: opts,
	}
}

// Name returns the preset's identifier.
func (vp *ValidationPreset) Name() string {
	return vp.name
}

// PresetMatcherFunc is a function that tests if token claims match a rule.
// Returns true if the claims satisfy the matching criteria.
//
// Example:
//
//	matcher := func(claims map[string]any) bool { return claims["role"] == "admin" }
type PresetMatcherFunc func(claims map[string]any) bool

// PresetRule represents a rule for automatic preset selection.
// Rules are evaluated in priority order (highest priority first).
type PresetRule struct {
	// Priority determines evaluation order (higher = checked first).
	Priority int

	// Matcher tests if this rule matches token claims.
	Matcher PresetMatcherFunc

	// PresetName is the preset to apply when matched.
	PresetName string
}

// compilePresets pre-compiles all registered presets during provider initialization.
func (p *Provider) compilePresets() {
	if len(p.opts.presets) == 0 {
		return
	}

	for _, preset := range p.opts.presets {
		// Apply preset options to create compiled verifier
		ops := &verifierOptions{}
		applyValidationOptions(ops, preset.options...)

		// Compile CEL rules if any and store separately
		preset.compiledCELRules = compileVerifierCELRules(ops, p.logger)

		// Pre-build ignored claims set for the compiled verifier
		ops.buildIgnoredSet()

		// Store compiled verifier in preset
		preset.compiledVerifier = ops
	}
}

// validatePresetSelectionRules filters invalid rules and sorts by priority.
func (p *Provider) validatePresetSelectionRules() {
	if len(p.opts.presetRules) == 0 {
		return
	}

	type indexedRule struct {
		rule  PresetRule
		index int
	}

	indexedRules := make([]indexedRule, 0, len(p.opts.presetRules))
	for idx, rule := range p.opts.presetRules {
		// Validate matcher function
		if rule.Matcher == nil {
			p.logger.Warn("skipping preset rule with nil matcher",
				"preset_name", rule.PresetName,
				"priority", rule.Priority)
			continue
		}
		// Validate preset name
		if rule.PresetName == "" {
			p.logger.Warn("skipping preset rule with empty preset name",
				"priority", rule.Priority)
			continue
		}
		indexedRules = append(indexedRules, indexedRule{
			rule:  rule,
			index: idx,
		})
	}

	// Sort rules by priority (highest first)
	slices.SortFunc(indexedRules, func(a, b indexedRule) int {
		if a.rule.Priority != b.rule.Priority {
			return b.rule.Priority - a.rule.Priority
		}

		switch {
		case a.index < b.index:
			return -1
		case a.index > b.index:
			return 1
		default:
			return 0
		}
	})

	p.opts.presetRules = make([]PresetRule, len(indexedRules))
	for i := range indexedRules {
		p.opts.presetRules[i] = indexedRules[i].rule
	}
}

// getPreset retrieves a preset by name, returning nil if not found.
func (p *Provider) getPreset(name string) *ValidationPreset {
	if p.opts.presets == nil {
		return nil
	}
	return p.opts.presets[name]
}

// ValidateTokenWithPreset validates a token using a named preset.
// Additional options override or extend the preset configuration.
//
// Example:
//
//	claims, _ := provider.ValidateTokenWithPreset(ctx, token, "strict")
func (p *Provider) ValidateTokenWithPreset(ctx context.Context, token string, presetName string, opt ...ValidationOption) (map[string]any, error) {
	preset := p.getPreset(presetName)
	if preset == nil {
		return nil, coreerrs.Wrapf(ErrInvalidToken, "validation preset '%s' not found", presetName)
	}

	// Fast path: use pre-compiled verifier if no additional options
	if len(opt) == 0 && preset.compiledVerifier != nil {
		return p.validateTokenWithPreset(ctx, token, preset)
	}

	// Combine preset options with additional options
	allOptions := make([]ValidationOption, 0, len(preset.options)+len(opt))
	allOptions = append(allOptions, preset.options...)
	allOptions = append(allOptions, opt...)

	return p.ValidateTokenWithOptions(ctx, token, allOptions...)
}

// validateTokenWithPreset validates a token using a pre-compiled preset.
func (p *Provider) validateTokenWithPreset(ctx context.Context, token string, preset *ValidationPreset) (map[string]any, error) {
	// Reject empty tokens immediately
	if token == "" {
		return nil, coreerrs.Wrap(ErrInvalidToken, "token is empty")
	}

	if err := p.checkTokenRevocation(ctx, token); err != nil {
		return nil, err
	}

	// Try to get cached claims if caching is enabled (only after revocation check)
	if p.tokenCache != nil {
		cacheKey := tokenCacheKey(p.opts.tokensCacheKeyPrefix, token)
		var claims map[string]any
		if err := p.tokenCache.Get(ctx, cacheKey, &claims); err == nil {
			return claims, nil
		}
	}

	// Verify signature first (without claim validation)
	claims, err := p.parseTokenWithoutClaimsValidation(token)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidToken, "signature verification failed: %v", err)
	}

	// Validate with pre-compiled verifier
	if err := p.validateWithPresetClaims(claims, preset.compiledVerifier, preset.compiledCELRules); err != nil {
		return nil, err
	}

	p.cacheValidatedClaims(ctx, token, claims)
	return claims, nil
}

// selectPresetForClaims returns the preset name matching claims, or empty string if none match.
func (p *Provider) selectPresetForClaims(claims map[string]any) string {
	// No rules configured
	if len(p.opts.presetRules) == 0 {
		return ""
	}

	// Try to find matching rule (rules are sorted by priority)
	for _, rule := range p.opts.presetRules {
		if rule.Matcher(claims) {
			return rule.PresetName
		}
	}

	// No rule matched - will use default validation options
	return ""
}
