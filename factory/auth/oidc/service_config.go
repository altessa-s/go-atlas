// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/ptr"
	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/oidc/rules"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	stdslices "slices"
)

// LoadServiceConfig loads and validates a ServiceConfig from a JSON file.
//
// Example:
//
//	config, _ := factory.LoadServiceConfig("./config/oidc-service.json")
func LoadServiceConfig(path string) (*authconfig.ServiceConfig, error) {
	// Defense-in-depth: resolve symlinks before reading so a symlinked config
	// path cannot silently redirect the read to an unexpected target. The path
	// still comes from trusted operator configuration.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "resolve config file path")
	}

	data, err := os.ReadFile(resolved) // #nosec G304 -- path comes from trusted configuration, symlinks resolved above
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read config file")
	}

	return authconfig.ParseServiceConfig(data)
}

// LoadServiceConfigOptions appends the file's policy after caller options. The
// returned options can be passed directly to oidc.NewProvider. A blank path
// keeps the caller options.
func LoadServiceConfigOptions(path string, caller ...oidc.Option) ([]oidc.Option, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return caller, nil
	}
	cfg, err := LoadServiceConfig(path)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "load service config")
	}
	overlay, err := ServiceConfigOptions(cfg)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "convert service config to options")
	}
	return stdslices.Concat(caller, overlay), nil
}

// ServiceConfigOptions converts ServiceConfig to provider Options.
func ServiceConfigOptions(c *authconfig.ServiceConfig) ([]oidc.Option, error) {
	var opts []oidc.Option

	// Default validation options
	if c.DefaultValidation != nil {
		validationOpts, err := ValidationOptions(c.DefaultValidation)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "convert default_validation")
		}

		opts = slices.AppendIf(opts, len(validationOpts) > 0, oidc.WithDefaultValidationOptions(validationOpts...))
	}

	// Presets
	if len(c.Presets) > 0 {
		var presets []*oidc.ValidationPreset

		for i := range c.Presets {
			presetDef := &c.Presets[i]

			validationOpts, err := ValidationOptions(presetDef.Validation)
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to convert preset %q", presetDef.Name)
			}

			preset := oidc.NewValidationPreset(presetDef.Name, validationOpts...)
			presets = append(presets, preset)
		}

		opts = append(opts, oidc.WithPresets(presets...))
	}

	// Preset rules
	if len(c.PresetRules) > 0 {
		var presetRules []oidc.PresetRule

		totalRules := len(c.PresetRules)
		for i := range c.PresetRules {
			ruleDef := &c.PresetRules[i]

			matcher, err := presetMatcher(ruleDef)
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to convert preset_rule[%d]", i)
			}

			presetRules = append(presetRules, oidc.PresetRule{
				PresetName: ruleDef.Preset,
				Matcher:    matcher,
				Priority:   totalRules - i, // Higher priority for earlier rules
			})
		}

		opts = append(opts, oidc.WithPresetRules(presetRules...))
	}

	return opts, nil
}

// ValidationOptions converts a service policy's validation block for use with
// oidc.NewValidationPreset or oidc.Provider.ValidateTokenWithOptions.
func ValidationOptions(v *authconfig.ValidationRulesConfig) ([]oidc.ValidationOption, error) {
	var opts []oidc.ValidationOption

	// All CEL rules (generated and custom) go into one oidc.WithValidationCelRules
	// call at the end: the option replaces earlier rules, so separate calls
	// would let the custom rules drop the generated ones.
	var celRules []oidc.CELValidationRule

	// Leeway
	if v.Leeway != "" {
		duration, err := time.ParseDuration(v.Leeway)
		if err != nil {
			return nil, coreerrs.Wrapf(authconfig.ErrInvalidDuration, "invalid leeway: %v", err)
		}
		opts = append(opts, oidc.WithValidationLeeway(duration))
	}

	// Verify not-before: nil = validated when present; true = required. An
	// explicit false is refused at config-validation time, so by the time
	// we reach this conversion it cannot reach the runtime path.
	opts = slices.AppendIf(opts, ptr.Unwrap(v.VerifyNotBefore), oidc.WithValidationNotBeforeRequired())

	// Verify issued at
	opts = slices.AppendIf(opts, ptr.Unwrap(v.VerifyIssuedAt), oidc.WithValidationIssuedAt())

	// Issuer
	opts = append(opts, oidc.WithValidationIssuer(v.Issuer))

	// Audiences
	opts = append(opts, oidc.WithValidationAudience(v.Audiences...))

	// Subject
	opts = append(opts, oidc.WithValidationSubject(v.Subject))

	// Required claims
	opts = append(opts, oidc.WithValidationRequiredClaims(v.RequiredClaims...))

	// Expected claims
	// Values keep their JSON types so the comparison stays type-aware.
	opts = slices.AppendIf(opts, len(v.ExpectedClaims) > 0, oidc.WithValidationExpectedClaims(maps.Clone(v.ExpectedClaims)))

	// Ignored claims
	opts = append(opts, oidc.WithValidationIgnoredClaims(v.IgnoredClaims...))

	// Allowed client IDs
	opts = append(opts, oidc.WithValidationAllowedClientIDs(v.AllowedClientIDs...))

	// Require authorized party
	opts = slices.AppendIf(opts, ptr.Unwrap(v.RequireAuthorizedParty) || len(v.AllowedAuthorizedParties) > 0,
		oidc.WithValidationRequireAuthorizedParty())

	// Allowed authorized parties
	opts = slices.AppendIf(opts, len(v.AllowedAuthorizedParties) > 0, oidc.WithValidationAllowedAuthorizedParties(v.AllowedAuthorizedParties...))

	// Allow missing subject
	opts = slices.AppendIf(opts, ptr.Unwrap(v.AllowMissingSubject), oidc.WithValidationAllowMissingSubject())

	// Scopes
	if v.Scopes != nil {
		// Combine all scope requirements into Required
		allScopes := make([]string, 0, len(v.Scopes.Required)+len(v.Scopes.AllOf))
		allScopes = append(allScopes, v.Scopes.Required...)
		allScopes = append(allScopes, v.Scopes.AllOf...)

		opts = slices.AppendIf(opts, len(allScopes) > 0, oidc.WithValidationRequiredScopes(allScopes...))

		// AnyOf: at least one of the specified scopes must be present
		// Implemented using native HasAnyScope matcher wrapped in CEL validation
		if len(v.Scopes.AnyOf) > 0 {
			// Build CEL expression: token must have at least one of the specified scopes
			scopeChecks := make([]string, len(v.Scopes.AnyOf))
			for i, scope := range v.Scopes.AnyOf {
				// Escape single quotes in scope names
				escapedScope := strings.ReplaceAll(scope, "'", "\\'")
				scopeChecks[i] = fmt.Sprintf("'%s'", escapedScope)
			}

			celExpr := fmt.Sprintf(
				"has(claims.scope) && ("+
					"(type(claims.scope) == string && [%s].exists(s, claims.scope.split(' ').exists(t, t == s))) || "+
					"(type(claims.scope) == list && [%s].exists(s, claims.scope.exists(t, t == s)))"+
					")",
				strings.Join(scopeChecks, ", "),
				strings.Join(scopeChecks, ", "),
			)

			celRules = append(celRules, oidc.CELValidationRule{
				Name:       "any-of-scopes",
				Expression: celExpr,
			})
		}
	}

	// Token lifetime
	if v.TokenLifetime != nil {
		var minLifetime, maxLifetime time.Duration
		var err error
		if v.TokenLifetime.Min != "" {
			minLifetime, err = time.ParseDuration(v.TokenLifetime.Min)
			if err != nil {
				return nil, coreerrs.Wrapf(authconfig.ErrInvalidDuration, "invalid token_lifetime.min: %v", err)
			}
		}
		if v.TokenLifetime.Max != "" {
			maxLifetime, err = time.ParseDuration(v.TokenLifetime.Max)
			if err != nil {
				return nil, coreerrs.Wrapf(authconfig.ErrInvalidDuration, "invalid token_lifetime.max: %v", err)
			}
		}
		if err := rules.CheckTokenLifetime(minLifetime, maxLifetime); err != nil {
			return nil, coreerrs.Wrap(err, "token_lifetime")
		}
		opts = slices.AppendIf(opts, v.TokenLifetime.Min != "", oidc.WithValidationMinTokenLifetime(minLifetime))
		opts = slices.AppendIf(opts, maxLifetime > 0, oidc.WithValidationMaxTokenLifetime(maxLifetime))
	}

	// CEL rules
	for i := range v.CELRules {
		ruleDef := &v.CELRules[i]

		celRules = append(celRules, oidc.CELValidationRule{
			Name:       ruleDef.Name,
			Expression: ruleDef.Expression,
			Message:    ruleDef.Message,
		})
	}
	opts = slices.AppendIf(opts, len(celRules) > 0, oidc.WithValidationCelRules(celRules...))

	return opts, nil
}

// presetMatcher compiles the validated condition into the matchers used for preset selection.
func presetMatcher(rule *authconfig.PresetRuleDefinition) (oidc.PresetMatcherFunc, error) {
	c, err := rule.ParseConditions()
	if err != nil {
		return nil, err
	}
	return compileCondition(c), nil
}

func compileCondition(c authconfig.PresetCondition) oidc.PresetMatcherFunc {
	var ms []oidc.PresetMatcherFunc
	switch c.Kind {
	case "claim_equals":
		for name, value := range c.Claims {
			ms = append(ms, oidc.ClaimValueEquals(name, value))
		}
	case "claim_contains":
		for name, substring := range c.Substrings {
			ms = append(ms, oidc.ClaimContains(name, substring))
		}
	case "claim_exists":
		for _, name := range c.Names {
			ms = append(ms, oidc.ClaimExists(name))
		}
	case "has_scope":
		return oidc.HasScope(c.Scope)
	case "has_any_scope":
		return oidc.HasAnyScope(c.Names...)
	case "has_all_scopes":
		return oidc.HasAllScopes(c.Names...)
	case "cel_expression":
		return c.CEL
	case "matcher_and", "matcher_or":
		for _, child := range c.Children {
			ms = append(ms, compileCondition(child))
		}
		if c.Kind == "matcher_or" {
			return oidc.MatcherOr(ms...)
		}
	default:
		panic("invalid parsed preset condition")
	}
	return oidc.MatcherAnd(ms...)
}
