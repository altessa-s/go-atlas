// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
)

const (
	// estimatedOptionsCount is the estimated number of provider options for capacity pre-allocation.
	estimatedOptionsCount = 5

	// estimatedValidationOptionsCount is the estimated number of validation options for capacity pre-allocation.
	estimatedValidationOptionsCount = 7
)

// validationOptionsFromConfig constructs validation options from configuration.
func validationOptionsFromConfig(cfg *config.OIDCValidation, clockSkew time.Duration) ([]oidc.ValidationOption, error) {
	if cfg == nil {
		return nil, fmt.Errorf("validation configuration is required")
	}

	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationMaxTokenLifetime(cfg.MaxTokenLifetime),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationIssuer(cfg.Issuer),
	)
	if cel := buildCELValidationRule(cfg.Expression, "custom-validation"); cel != nil {
		opts = append(opts, cel)
	}

	opts = slices.AppendIf(opts, cfg.IsClaimsConfigured(), buildClaimsValidationOptions(cfg.Claims)...)

	return opts, nil
}

// presetFromConfig creates a validation preset from configuration.
func presetFromConfig(preset *config.OIDCPreset, clockSkew time.Duration) (*oidc.ValidationPreset, error) {
	if preset == nil {
		return nil, fmt.Errorf("preset configuration is required")
	}
	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationMaxTokenLifetime(preset.MaxTokenLifetime),
		oidc.WithValidationIssuer(preset.Issuer),
	)
	opts = slices.AppendIf(opts, preset.IsClaimsConfigured(), buildClaimsValidationOptions(preset.Claims)...)
	if cel := buildCELValidationRule(preset.Expression, preset.Name+"-validation"); cel != nil {
		opts = append(opts, cel)
	}

	return oidc.NewValidationPreset(preset.Name, opts...), nil
}

// presetRuleFromConfig creates a preset selection rule from configuration.
func presetRuleFromConfig(ctx context.Context, selector *config.OIDCSelector) oidc.PresetRule {
	return oidc.PresetRule{
		Priority:   selector.Priority,
		Matcher:    oidc.CELMatcher(ctx, selector.Expression),
		PresetName: selector.PresetName,
	}
}

// buildCELValidationRule creates a CEL validation rule from expression configuration.
func buildCELValidationRule(expr *config.OIDCExpression, defaultName string) oidc.ValidationOption {
	if expr == nil || expr.Expression == "" {
		return nil
	}

	rule := oidc.CELValidationRule{
		Name:       cmp.Or(expr.Name, defaultName),
		Expression: expr.Expression,
	}

	return oidc.WithValidationCelRules(rule)
}

// buildClaimsValidationOptions creates validation options from claims configuration.
func buildClaimsValidationOptions(claims *config.OIDCClaims) []oidc.ValidationOption {
	if claims == nil {
		return nil
	}

	opts := []oidc.ValidationOption{
		oidc.WithValidationAudience(claims.Audience...),
		oidc.WithValidationRequiredClaims(claims.Required...),
		oidc.WithValidationExpectedClaims(claims.Expected),
		oidc.WithValidationIgnoredClaims(claims.Ignored...),
		oidc.WithValidationAllowedClientIDs(claims.AllowedClientIds...),
		oidc.WithValidationRequiredScopes(claims.RequiredScopes...),
	}

	opts = slices.AppendIf(opts, claims.AllowMissingSubject, oidc.WithValidationAllowMissingSubject())

	if claims.RequireAuthorizedParty && len(claims.AllowedAuthorizedParties) > 0 {
		opts = append(opts,
			oidc.WithValidationRequireAuthorizedParty(),
			oidc.WithValidationAllowedAuthorizedParties(claims.AllowedAuthorizedParties...))
	}

	return opts
}
