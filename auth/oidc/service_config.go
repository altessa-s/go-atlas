// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/ptr"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// SupportedConfigVersion is the current supported configuration version.
const SupportedConfigVersion = "1.0"

// ServiceConfig represents OIDC validation configuration loaded from JSON.
// Supports default validation, presets, and automatic preset selection rules.
type ServiceConfig struct {
	// Version is the configuration schema version (currently "1.0").
	Version string `json:"version"`

	// DefaultValidation contains default validation options applied to all tokens
	// without a specific preset (optional).
	DefaultValidation *ValidationRulesConfig `json:"default_validation,omitempty"`

	// Presets defines named validation presets for different use cases (optional).
	Presets []PresetDefinition `json:"presets,omitempty"`

	// PresetRules defines automatic preset selection rules based on token claims (optional).
	PresetRules []PresetRuleDefinition `json:"preset_rules,omitempty"`
}

// ValidationRulesConfig defines token validation rules for JSON configuration.
type ValidationRulesConfig struct {
	// Leeway is the clock skew tolerance for time-based claims (exp, nbf, iat).
	// Format: Go duration string (e.g., "10s", "30s").
	// Default: "0s"
	Leeway string `json:"leeway,omitempty"`

	// VerifyNotBefore controls verification of token not-before time (nbf claim).
	//
	// Tri-state:
	//   - nil (field omitted): nbf is validated when present, missing nbf
	//     is accepted.
	//   - true: nbf is required AND validated. Equivalent to passing
	//     [WithValidationNotBeforeRequired] at runtime.
	//   - false: REJECTED at config-load time with [ErrInvalidConfig], so a
	//     misconfiguration stays loud instead of silently inverting intent.
	//
	// The exp claim has no such switch: it is always required and validated.
	VerifyNotBefore *bool `json:"verify_not_before,omitempty"`

	// VerifyIssuedAt enables verification of token issued-at time (iat claim).
	// Default: false
	VerifyIssuedAt *bool `json:"verify_issued_at,omitempty"`

	// Issuer is the expected token issuer (iss claim).
	Issuer string `json:"issuer,omitempty"`

	// Audiences are the expected token audiences (aud claim).
	// Token must contain at least one of these audiences.
	Audiences []string `json:"audiences,omitempty"`

	// Subject is the expected token subject (sub claim).
	Subject string `json:"subject,omitempty"`

	// RequiredClaims are claims that must be present in the token.
	RequiredClaims []string `json:"required_claims,omitempty"`

	// ExpectedClaims are claims with expected exact values, compared by JSON
	// type (a bool expectation is not satisfied by the string "true"); an
	// array claim matches when any element matches.
	ExpectedClaims map[string]any `json:"expected_claims,omitempty"`

	// IgnoredClaims are claims to ignore during validation.
	IgnoredClaims []string `json:"ignored_claims,omitempty"`

	// AllowedClientIDs restricts which client_id values are allowed.
	// Empty list allows all client IDs. Used for service-to-service authentication.
	AllowedClientIDs []string `json:"allowed_client_ids,omitempty"`

	// RequireAuthorizedParty requires 'azp' (authorized party) claim presence.
	// Useful for multi-tenant scenarios. Default: false
	RequireAuthorizedParty *bool `json:"require_authorized_party,omitempty"`

	// AllowedAuthorizedParties restricts allowed 'azp' values.
	// Only checked if RequireAuthorizedParty is true or azp claim is present.
	AllowedAuthorizedParties []string `json:"allowed_authorized_parties,omitempty"`

	// AllowMissingSubject allows tokens without 'sub' claim.
	// Useful for service account tokens (client credentials flow). Default: false
	AllowMissingSubject *bool `json:"allow_missing_subject,omitempty"`

	// Scopes configures OAuth2 scope validation.
	Scopes *ScopesValidationConfig `json:"scopes,omitempty"`

	// TokenLifetime configures token lifetime constraints.
	TokenLifetime *TokenLifetimeValidationConfig `json:"token_lifetime,omitempty"`

	// CELRules are custom CEL (Common Expression Language) validation rules.
	CELRules []CELRuleDefinition `json:"cel_rules,omitempty"`
}

// ScopesValidationConfig defines scope validation rules.
type ScopesValidationConfig struct {
	Required []string `json:"required,omitempty"`
	AnyOf    []string `json:"any_of,omitempty"`
	AllOf    []string `json:"all_of,omitempty"`
}

// TokenLifetimeValidationConfig defines token lifetime constraints.
type TokenLifetimeValidationConfig struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

// CELRuleDefinition represents a CEL validation rule in JSON configuration.
type CELRuleDefinition struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Message    string `json:"message,omitempty"`
}

// PresetDefinition defines a validation preset in JSON configuration.
type PresetDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Validation  *ValidationRulesConfig `json:"validation"`
}

// PresetRuleDefinition defines a preset selection rule in JSON configuration.
type PresetRuleDefinition struct {
	Description string         `json:"description,omitempty"`
	Preset      string         `json:"preset"`
	Conditions  map[string]any `json:"conditions"`
}

var (
	// ErrInvalidConfig indicates invalid service configuration.
	ErrInvalidConfig = errors.New("invalid service configuration")

	// ErrInvalidVersion indicates unsupported configuration version.
	ErrInvalidVersion = errors.New("unsupported configuration version")

	// ErrInvalidDuration indicates invalid duration string format.
	ErrInvalidDuration = errors.New("invalid duration format")

	// ErrPresetNotFound indicates a referenced preset does not exist.
	ErrPresetNotFound = errors.New("preset not found")

	// ErrInvalidCEL indicates an invalid CEL expression.
	ErrInvalidCEL = errors.New("invalid CEL expression")
)

// LoadServiceConfig loads and validates a ServiceConfig from a JSON file.
//
// Example:
//
//	config, _ := oidc.LoadServiceConfig("./config/oidc-service.json")
func LoadServiceConfig(path string) (*ServiceConfig, error) {
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

	return ParseServiceConfig(data)
}

// ParseServiceConfig parses and validates a ServiceConfig from JSON bytes.
//
// Example:
//
//	config, _ := oidc.ParseServiceConfig(jsonData)
func ParseServiceConfig(data []byte) (*ServiceConfig, error) {
	var config ServiceConfig

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidConfig, "failed to parse JSON: %v", err)
	}

	if err := ValidateServiceConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// ValidateServiceConfig validates a ServiceConfig structure including CEL expressions.
func ValidateServiceConfig(config *ServiceConfig) error {
	// Validate required fields
	if config.Version == "" {
		return coreerrs.Wrap(ErrInvalidConfig, "version is required")
	}

	if config.Version != SupportedConfigVersion {
		return coreerrs.Wrapf(ErrInvalidVersion, "version %q not supported (expected %q)",
			config.Version, SupportedConfigVersion)
	}

	// Validate default validation config
	if config.DefaultValidation != nil {
		if err := validateValidationRulesConfig(config.DefaultValidation, "default_validation"); err != nil {
			return err
		}
	}

	// Validate presets
	presetNames := make(map[string]bool)
	for i := range config.Presets {
		preset := &config.Presets[i]

		if preset.Name == "" {
			return coreerrs.Wrapf(ErrInvalidConfig, "presets[%d].name is required", i)
		}

		if presetNames[preset.Name] {
			return coreerrs.Wrapf(ErrInvalidConfig, "duplicate preset name %q", preset.Name)
		}
		presetNames[preset.Name] = true

		if preset.Validation == nil {
			return coreerrs.Wrapf(ErrInvalidConfig, "presets[%d].validation is required", i)
		}

		if err := validateValidationRulesConfig(preset.Validation, fmt.Sprintf("presets[%d].validation", i)); err != nil {
			return err
		}
	}

	// Validate preset rules
	for i := range config.PresetRules {
		rule := &config.PresetRules[i]

		if rule.Preset == "" {
			return coreerrs.Wrapf(ErrInvalidConfig, "preset_rules[%d].preset is required", i)
		}

		if !presetNames[rule.Preset] {
			return coreerrs.Wrapf(ErrPresetNotFound, "preset_rules[%d] references unknown preset %q",
				i, rule.Preset)
		}

		if len(rule.Conditions) == 0 {
			return coreerrs.Wrapf(ErrInvalidConfig, "preset_rules[%d].conditions is required", i)
		}

		if _, err := conditionsToMatcher(rule.Conditions); err != nil {
			return coreerrs.Wrapf(err, "preset_rules[%d].conditions", i)
		}
	}

	return nil
}

// ToProviderOptions converts ServiceConfig to provider Options.
func (c *ServiceConfig) ToProviderOptions() ([]Option, error) {
	var opts []Option

	// Default validation options
	if c.DefaultValidation != nil {
		validationOpts, err := c.DefaultValidation.ToValidationOptions()
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "convert default_validation")
		}

		opts = slices.AppendIf(opts, len(validationOpts) > 0, WithDefaultValidationOptions(validationOpts...))
	}

	// Presets
	if len(c.Presets) > 0 {
		var presets []*ValidationPreset

		for i := range c.Presets {
			presetDef := &c.Presets[i]

			validationOpts, err := presetDef.Validation.ToValidationOptions()
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to convert preset %q", presetDef.Name)
			}

			preset := NewValidationPreset(presetDef.Name, validationOpts...)
			presets = append(presets, preset)
		}

		opts = append(opts, WithPresets(presets...))
	}

	// Preset rules
	if len(c.PresetRules) > 0 {
		var presetRules []PresetRule

		totalRules := len(c.PresetRules)
		for i := range c.PresetRules {
			ruleDef := &c.PresetRules[i]

			matcher, err := conditionsToMatcher(ruleDef.Conditions)
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to convert preset_rule[%d]", i)
			}

			presetRules = append(presetRules, PresetRule{
				PresetName: ruleDef.Preset,
				Matcher:    matcher,
				Priority:   totalRules - i, // Higher priority for earlier rules
			})
		}

		opts = append(opts, WithPresetRules(presetRules...))
	}

	return opts, nil
}

// ToValidationOptions converts ValidationRulesConfig to a slice of ValidationOption.
func (v *ValidationRulesConfig) ToValidationOptions() ([]ValidationOption, error) {
	var opts []ValidationOption

	// All CEL rules (generated and custom) go into one WithValidationCelRules
	// call at the end: the option replaces earlier rules, so separate calls
	// would let the custom rules drop the generated ones.
	var celRules []CELValidationRule

	// Leeway
	if v.Leeway != "" {
		duration, err := time.ParseDuration(v.Leeway)
		if err != nil {
			return nil, coreerrs.Wrapf(ErrInvalidDuration, "invalid leeway: %v", err)
		}
		opts = append(opts, WithValidationLeeway(duration))
	}

	// Verify not-before: nil = validated when present; true = required. An
	// explicit false is refused at config-validation time, so by the time
	// we reach this conversion it cannot reach the runtime path.
	opts = slices.AppendIf(opts, ptr.Unwrap(v.VerifyNotBefore), WithValidationNotBeforeRequired())

	// Verify issued at
	opts = slices.AppendIf(opts, ptr.Unwrap(v.VerifyIssuedAt), WithValidationIssuedAt())

	// Issuer
	opts = append(opts, WithValidationIssuer(v.Issuer))

	// Audiences
	opts = append(opts, WithValidationAudience(v.Audiences...))

	// Subject
	opts = append(opts, WithValidationSubject(v.Subject))

	// Required claims
	opts = append(opts, WithValidationRequiredClaims(v.RequiredClaims...))

	// Expected claims
	// Values keep their JSON types so the comparison stays type-aware.
	opts = slices.AppendIf(opts, len(v.ExpectedClaims) > 0, WithValidationExpectedClaims(maps.Clone(v.ExpectedClaims)))

	// Ignored claims
	opts = append(opts, WithValidationIgnoredClaims(v.IgnoredClaims...))

	// Allowed client IDs
	opts = append(opts, WithValidationAllowedClientIDs(v.AllowedClientIDs...))

	// Require authorized party
	opts = slices.AppendIf(opts, ptr.Unwrap(v.RequireAuthorizedParty), WithValidationRequireAuthorizedParty())

	// Allowed authorized parties
	opts = slices.AppendIf(opts, len(v.AllowedAuthorizedParties) > 0, WithValidationAllowedAuthorizedParties(v.AllowedAuthorizedParties...))

	// Allow missing subject
	opts = slices.AppendIf(opts, ptr.Unwrap(v.AllowMissingSubject), WithValidationAllowMissingSubject())

	// Scopes
	if v.Scopes != nil {
		// Combine all scope requirements into Required
		allScopes := make([]string, 0, len(v.Scopes.Required)+len(v.Scopes.AllOf))
		allScopes = append(allScopes, v.Scopes.Required...)
		allScopes = append(allScopes, v.Scopes.AllOf...)

		opts = slices.AppendIf(opts, len(allScopes) > 0, WithValidationRequiredScopes(allScopes...))

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

			celRules = append(celRules, CELValidationRule{
				Name:       "any-of-scopes",
				Expression: celExpr,
			})
		}
	}

	// Token lifetime
	if v.TokenLifetime != nil {
		// Only max lifetime is supported for now
		// Min lifetime would need custom CEL validation or a new function

		if v.TokenLifetime.Max != "" {
			maxDuration, err := time.ParseDuration(v.TokenLifetime.Max)
			if err != nil {
				return nil, coreerrs.Wrapf(ErrInvalidDuration, "invalid token_lifetime.max: %v", err)
			}

			opts = slices.AppendIf(opts, maxDuration > 0, WithValidationMaxTokenLifetime(maxDuration))
		}

		// Note: Min lifetime is not directly supported
		// It would need custom CEL validation
		_ = v.TokenLifetime.Min
	}

	// CEL rules
	for i := range v.CELRules {
		ruleDef := &v.CELRules[i]

		celRules = append(celRules, CELValidationRule{
			Name:       ruleDef.Name,
			Expression: ruleDef.Expression,
			Message:    ruleDef.Message,
		})
	}
	opts = slices.AppendIf(opts, len(celRules) > 0, WithValidationCelRules(celRules...))

	return opts, nil
}

// conditionsToMatcher converts JSON condition map to a PresetMatcherFunc.
func conditionsToMatcher(conditions map[string]any) (PresetMatcherFunc, error) {
	if len(conditions) != 1 {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "conditions must contain exactly one matcher")
	}

	var key string
	var value any
	for k, v := range conditions {
		key, value = k, v
	}

	switch key {
	case "claim_equals":
		return claimEqualsMatcher(value)
	case "claim_exists":
		return claimExistsMatcher(value)
	case "claim_contains":
		return claimContainsMatcher(value)
	case "has_scope":
		return hasScopeMatcher(value)
	case "has_any_scope":
		scopes, err := parseNonEmptyStringArray(key, value)
		if err != nil {
			return nil, err
		}
		return HasAnyScope(scopes...), nil
	case "has_all_scopes":
		scopes, err := parseNonEmptyStringArray(key, value)
		if err != nil {
			return nil, err
		}
		return HasAllScopes(scopes...), nil
	case "cel_expression":
		return celExpressionMatcher(value)
	case "matcher_and":
		return combinedMatcher(key, value, MatcherAnd)
	case "matcher_or":
		return combinedMatcher(key, value, MatcherOr)
	}

	return nil, coreerrs.Wrapf(ErrInvalidConfig, "unknown condition type %q", key)
}

// claimEqualsMatcher requires every listed claim to equal its expected value.
func claimEqualsMatcher(value any) (PresetMatcherFunc, error) {
	typed, ok := value.(map[string]any)
	if !ok || len(typed) == 0 {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "claim_equals requires a non-empty object")
	}

	matchers := make([]PresetMatcherFunc, 0, len(typed))
	for claimKey, expected := range typed {
		matchers = append(matchers, func(claims map[string]any) bool {
			claimValue, exists := claims[claimKey]
			return exists && conditionValueEquals(claimValue, expected)
		})
	}

	return MatcherAnd(matchers...), nil
}

// conditionValueEquals compares a claim with a claim_equals value: strings,
// numbers, bools and null by JSON type; any other value by its %v text.
func conditionValueEquals(actual, expected any) bool {
	switch exp := expected.(type) {
	case string:
		got, ok := actual.(string)
		return ok && got == exp
	case float64:
		got, ok := actual.(float64)
		return ok && got == exp
	case bool:
		got, ok := actual.(bool)
		return ok && got == exp
	case nil:
		return actual == nil
	default:
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
	}
}

// claimExistsMatcher requires every listed claim to be present.
func claimExistsMatcher(value any) (PresetMatcherFunc, error) {
	rawList, ok := value.([]any)
	if !ok || len(rawList) == 0 {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "claim_exists requires a non-empty array")
	}

	claimNames := make([]string, len(rawList))
	for i, item := range rawList {
		name, ok := item.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, coreerrs.Wrap(ErrInvalidConfig, "claim_exists array elements must be non-empty strings")
		}
		claimNames[i] = name
	}

	return func(claims map[string]any) bool {
		for _, name := range claimNames {
			if _, exists := claims[name]; !exists {
				return false
			}
		}
		return true
	}, nil
}

// claimContainsMatcher requires every listed claim to contain its substring.
func claimContainsMatcher(value any) (PresetMatcherFunc, error) {
	typed, ok := value.(map[string]any)
	if !ok || len(typed) == 0 {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "claim_contains requires a non-empty object")
	}

	matchers := make([]PresetMatcherFunc, 0, len(typed))
	for claimKey, rawVal := range typed {
		substring, ok := rawVal.(string)
		if !ok {
			return nil, coreerrs.Wrap(ErrInvalidConfig, "claim_contains values must be strings")
		}
		matchers = append(matchers, ClaimContains(claimKey, substring))
	}

	return MatcherAnd(matchers...), nil
}

// hasScopeMatcher requires one scope.
func hasScopeMatcher(value any) (PresetMatcherFunc, error) {
	scope, ok := value.(string)
	if !ok || strings.TrimSpace(scope) == "" {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "has_scope requires a non-empty string")
	}
	return HasScope(scope), nil
}

// celExpressionMatcher compiles a CEL expression evaluated against the claims.
func celExpressionMatcher(value any) (PresetMatcherFunc, error) {
	expression, ok := value.(string)
	if !ok || strings.TrimSpace(expression) == "" {
		return nil, coreerrs.Wrap(ErrInvalidConfig, "cel_expression requires a non-empty string")
	}

	program, err := compileCELExpression(expression)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCEL, "%v", err)
	}

	if cache := getCELCache(); cache != nil {
		cache.Put(expression, program)
	}

	return func(claims map[string]any) bool {
		return evaluateCELProgram(program, claims)
	}, nil
}

// combinedMatcher compiles a matcher_and / matcher_or list of nested
// conditions and joins them with combine.
func combinedMatcher(key string, value any, combine func(...PresetMatcherFunc) PresetMatcherFunc) (PresetMatcherFunc, error) {
	rawList, ok := value.([]any)
	if !ok || len(rawList) == 0 {
		return nil, coreerrs.Wrapf(ErrInvalidConfig, "%s requires a non-empty array", key)
	}

	matchers := make([]PresetMatcherFunc, 0, len(rawList))
	for idx, item := range rawList {
		condMap, ok := item.(map[string]any)
		if !ok {
			return nil, coreerrs.Wrapf(ErrInvalidConfig, "%s[%d] must be an object", key, idx)
		}

		matcher, err := conditionsToMatcher(condMap)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "%s[%d]", key, idx)
		}
		matchers = append(matchers, matcher)
	}

	return combine(matchers...), nil
}

func parseNonEmptyStringArray(key string, value any) ([]string, error) {
	rawList, ok := value.([]any)
	if !ok || len(rawList) == 0 {
		return nil, coreerrs.Wrapf(ErrInvalidConfig, "%s requires a non-empty array", key)
	}

	out := make([]string, len(rawList))
	for i, item := range rawList {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, coreerrs.Wrapf(ErrInvalidConfig, "%s array elements must be non-empty strings", key)
		}
		out[i] = s
	}
	return out, nil
}

// validateValidationRulesConfig validates a ValidationRulesConfig including CEL rules.
func validateValidationRulesConfig(config *ValidationRulesConfig, path string) error {
	// Validate leeway
	if config.Leeway != "" {
		leeway, err := time.ParseDuration(config.Leeway)
		if err != nil {
			return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.leeway: %v", path, err)
		}
		if err := checkLeeway(leeway); err != nil {
			return coreerrs.Wrapf(err, "%s.leeway", path)
		}
	}

	// Reject an explicit `false` on the verify_not_before switch. The
	// validator has no per-claim opt-out, so silently honoring `false` here
	// would leave operators thinking they disabled a check that is still
	// running. Refusing the value at config-load time keeps the
	// misconfiguration loud.
	if config.VerifyNotBefore != nil && !*config.VerifyNotBefore {
		return coreerrs.Wrapf(ErrInvalidConfig,
			"%s.verify_not_before: explicit `false` is not supported — omit the field to keep the JWT default or set it to true to require the nbf claim",
			path)
	}

	// Validate token lifetime
	if config.TokenLifetime != nil {
		if config.TokenLifetime.Min != "" {
			if _, err := time.ParseDuration(config.TokenLifetime.Min); err != nil {
				return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.token_lifetime.min: %v", path, err)
			}
		}

		if config.TokenLifetime.Max != "" {
			if _, err := time.ParseDuration(config.TokenLifetime.Max); err != nil {
				return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.token_lifetime.max: %v", path, err)
			}
		}
	}

	// Validate CEL rules
	for i := range config.CELRules {
		rule := &config.CELRules[i]

		if rule.Name == "" {
			return coreerrs.Wrapf(ErrInvalidConfig, "%s.cel_rules[%d].name is required", path, i)
		}

		if rule.Expression == "" {
			return coreerrs.Wrapf(ErrInvalidConfig, "%s.cel_rules[%d].expression is required", path, i)
		}

		if _, err := compileCELExpression(rule.Expression); err != nil {
			return coreerrs.Wrapf(ErrInvalidCEL, "invalid %s.cel_rules[%d].expression: %v", path, i, err)
		}
	}

	return nil
}
