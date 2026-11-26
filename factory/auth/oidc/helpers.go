// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/auth/oidc"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

const (
	// estimatedOptionsCount is the estimated number of provider options for capacity pre-allocation.
	estimatedOptionsCount = 5

	// estimatedValidationOptionsCount is the estimated number of validation options for capacity pre-allocation.
	estimatedValidationOptionsCount = 8
)

// validationOptionsFromConfig constructs validation options from configuration.
func validationOptionsFromConfig(cfg *authconfig.OIDCValidation, clockSkew time.Duration) ([]oidc.ValidationOption, error) {
	if cfg == nil {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "validation configuration is required")
	}

	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationMinTokenLifetime(cfg.MinTokenLifetime),
		oidc.WithValidationMaxTokenLifetime(cfg.MaxTokenLifetime),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationIssuer(cfg.Issuer),
	)
	if cel := buildCELValidationRule(cfg.Expression, "custom-validation"); cel != nil {
		opts = append(opts, cel)
	}

	claimsOpts, err := buildClaimsValidationOptions(cfg.Claims)
	if err != nil {
		return nil, err
	}
	return append(opts, claimsOpts...), nil
}

// presetFromConfig creates a validation preset from configuration.
func presetFromConfig(preset *authconfig.OIDCPreset, clockSkew time.Duration) (*oidc.ValidationPreset, error) {
	if preset == nil {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "preset configuration is required")
	}
	opts := make([]oidc.ValidationOption, 0, estimatedValidationOptionsCount)
	opts = append(opts,
		oidc.WithValidationIssuedAt(),
		oidc.WithValidationLeeway(clockSkew),
		oidc.WithValidationMinTokenLifetime(preset.MinTokenLifetime),
		oidc.WithValidationMaxTokenLifetime(preset.MaxTokenLifetime),
		oidc.WithValidationIssuer(preset.Issuer),
	)
	claimsOpts, err := buildClaimsValidationOptions(preset.Claims)
	if err != nil {
		return nil, err
	}
	opts = append(opts, claimsOpts...)
	if cel := buildCELValidationRule(preset.Expression, preset.Name+"-validation"); cel != nil {
		opts = append(opts, cel)
	}

	return oidc.NewValidationPreset(preset.Name, opts...), nil
}

// presetRuleFromConfig creates a preset selection rule from configuration.
func presetRuleFromConfig(ctx context.Context, selector *authconfig.OIDCSelector) oidc.PresetRule {
	return oidc.PresetRule{
		Priority:   selector.Priority,
		Matcher:    oidc.CELMatcher(ctx, selector.Expression),
		PresetName: selector.PresetName,
	}
}

// buildCELValidationRule creates a CEL validation rule from expression configuration.
func buildCELValidationRule(expr *authconfig.OIDCExpression, defaultName string) oidc.ValidationOption {
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
func buildClaimsValidationOptions(claims *authconfig.OIDCClaims) ([]oidc.ValidationOption, error) {
	if claims == nil {
		return nil, nil
	}
	if err := validateAuthorizedParties(claims.AllowedAuthorizedParties, coreerrs.ErrInvalidArgument); err != nil {
		return nil, err
	}

	opts := []oidc.ValidationOption{
		oidc.WithValidationAudience(claims.Audience...),
		oidc.WithValidationRequiredClaims(claims.Required...),
		// YAML expectations are strings; a string expectation also matches a
		// bool or number claim with the same canonical text.
		oidc.WithValidationExpectedClaims(coremaps.ConvertMap(claims.Expected, func(k, v string) (string, any) { return k, v })),
		oidc.WithValidationIgnoredClaims(claims.Ignored...),
		oidc.WithValidationAllowedClientIDs(claims.AllowedClientIds...),
		oidc.WithValidationRequiredScopes(claims.RequiredScopes...),
	}

	opts = slices.AppendIf(opts, claims.AllowMissingSubject, oidc.WithValidationAllowMissingSubject())

	opts = slices.AppendIf(opts, claims.RequireAuthorizedParty || len(claims.AllowedAuthorizedParties) > 0,
		oidc.WithValidationRequireAuthorizedParty())
	opts = slices.AppendIf(opts, len(claims.AllowedAuthorizedParties) > 0,
		oidc.WithValidationAllowedAuthorizedParties(claims.AllowedAuthorizedParties...))

	return opts, nil
}

func validateAuthorizedParties(parties []string, kind error) error {
	for i, party := range parties {
		if strings.TrimSpace(party) == "" {
			return coreerrs.Wrapf(kind, "authorized party at index %d must not be blank", i)
		}
	}
	return nil
}
