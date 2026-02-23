// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
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

	// VerifyExpiration enables verification of token expiration time (exp claim).
	// Default: true
	VerifyExpiration *bool `json:"verify_expiration,omitempty"`

	// VerifyNotBefore enables verification of token not-before time (nbf claim).
	// Default: false
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

	// ExpectedClaims are claims with expected exact values.
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
//
// #nosec G304 -- path comes from trusted configuration
func LoadServiceConfig(path string) (*ServiceConfig, error) {
	data, err := os.ReadFile(path)
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
		return nil, fmt.Errorf("%w: failed to parse JSON: %v", ErrInvalidConfig, err)
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
		return fmt.Errorf("%w: version is required", ErrInvalidConfig)
	}

	if config.Version != SupportedConfigVersion {
		return fmt.Errorf("%w: version %q not supported (expected %q)",
			ErrInvalidVersion, config.Version, SupportedConfigVersion)
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
			return fmt.Errorf("%w: presets[%d].name is required", ErrInvalidConfig, i)
		}

		if presetNames[preset.Name] {
			return fmt.Errorf("%w: duplicate preset name %q", ErrInvalidConfig, preset.Name)
		}
		presetNames[preset.Name] = true

		if preset.Validation == nil {
			return fmt.Errorf("%w: presets[%d].validation is required", ErrInvalidConfig, i)
		}

		if err := validateValidationRulesConfig(preset.Validation, fmt.Sprintf("presets[%d].validation", i)); err != nil {
			return err
		}
	}

	// Validate preset rules
	for i := range config.PresetRules {
		rule := &config.PresetRules[i]

		if rule.Preset == "" {
			return fmt.Errorf("%w: preset_rules[%d].preset is required", ErrInvalidConfig, i)
		}

		if !presetNames[rule.Preset] {
			return fmt.Errorf("%w: preset_rules[%d] references unknown preset %q",
				ErrPresetNotFound, i, rule.Preset)
		}

		if len(rule.Conditions) == 0 {
			return fmt.Errorf("%w: preset_rules[%d].conditions is required", ErrInvalidConfig, i)
		}

		if _, err := conditionsToMatcher(rule.Conditions); err != nil {
			return fmt.Errorf("preset_rules[%d].conditions: %w", i, err)
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

		if len(validationOpts) > 0 {
			opts = append(opts, WithDefaultValidationOptions(validationOpts...))
		}
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

	// Leeway
	if v.Leeway != "" {
		duration, err := time.ParseDuration(v.Leeway)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid leeway: %v", ErrInvalidDuration, err)
		}
		opts = append(opts, WithValidationLeeway(duration))
	}

	// Note: VerifyExpiration and VerifyNotBefore are handled automatically by JWT library
	// We only support WithValidationIssuedAt() for now

	// Verify issued at
	if v.VerifyIssuedAt != nil {
		if *v.VerifyIssuedAt {
			opts = append(opts, WithValidationIssuedAt())
		}
	}

	// Issuer
	opts = append(opts, WithValidationIssuer(v.Issuer))

	// Audiences
	opts = append(opts, WithValidationAudience(v.Audiences...))

	// Subject
	opts = append(opts, WithValidationSubject(v.Subject))

	// Required claims
	opts = append(opts, WithValidationRequiredClaims(v.RequiredClaims...))

	// Expected claims
	if len(v.ExpectedClaims) > 0 {
		// Convert map[string]any to map[string]string
		expectedStr := make(map[string]string)
		for key, value := range v.ExpectedClaims {
			if strValue, ok := value.(string); ok {
				expectedStr[key] = strValue
			} else {
				// Convert non-string values to string representation
				expectedStr[key] = fmt.Sprintf("%v", value)
			}
		}
		opts = append(opts, WithValidationExpectedClaims(expectedStr))
	}

	// Ignored claims
	opts = append(opts, WithValidationIgnoredClaims(v.IgnoredClaims...))

	// Allowed client IDs
	opts = append(opts, WithValidationAllowedClientIDs(v.AllowedClientIDs...))

	// Require authorized party
	if v.RequireAuthorizedParty != nil && *v.RequireAuthorizedParty {
		opts = append(opts, WithValidationRequireAuthorizedParty())
	}

	// Allowed authorized parties
	if len(v.AllowedAuthorizedParties) > 0 {
		opts = append(opts, WithValidationAllowedAuthorizedParties(v.AllowedAuthorizedParties...))
	}

	// Allow missing subject
	if v.AllowMissingSubject != nil && *v.AllowMissingSubject {
		opts = append(opts, WithValidationAllowMissingSubject())
	}

	// Scopes
	if v.Scopes != nil {
		// Combine all scope requirements into Required
		allScopes := make([]string, 0, len(v.Scopes.Required)+len(v.Scopes.AllOf))
		allScopes = append(allScopes, v.Scopes.Required...)
		allScopes = append(allScopes, v.Scopes.AllOf...)

		if len(allScopes) > 0 {
			opts = append(opts, WithValidationRequiredScopes(allScopes...))
		}

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

			opts = append(opts, WithValidationCelRules(CELValidationRule{
				Name:       "any-of-scopes",
				Expression: celExpr,
			}))
		}
	}

	// Token lifetime
	if v.TokenLifetime != nil {
		// Only max lifetime is supported for now
		// Min lifetime would need custom CEL validation or a new function

		if v.TokenLifetime.Max != "" {
			maxDuration, err := time.ParseDuration(v.TokenLifetime.Max)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid token_lifetime.max: %v", ErrInvalidDuration, err)
			}

			if maxDuration > 0 {
				opts = append(opts, WithValidationMaxTokenLifetime(maxDuration))
			}
		}

		// Note: Min lifetime is not directly supported
		// It would need custom CEL validation
		_ = v.TokenLifetime.Min
	}

	// CEL rules
	if len(v.CELRules) > 0 {
		var celRules []CELValidationRule

		for i := range v.CELRules {
			ruleDef := &v.CELRules[i]

			celRules = append(celRules, CELValidationRule{
				Name:       ruleDef.Name,
				Expression: ruleDef.Expression,
				Message:    ruleDef.Message,
			})
		}

		opts = append(opts, WithValidationCelRules(celRules...))
	}

	return opts, nil
}

// conditionsToMatcher converts JSON condition map to a PresetMatcherFunc.
func conditionsToMatcher(conditions map[string]any) (PresetMatcherFunc, error) {
	if len(conditions) != 1 {
		return nil, fmt.Errorf("%w: conditions must contain exactly one matcher", ErrInvalidConfig)
	}

	var key string
	var value any
	for k, v := range conditions {
		key, value = k, v
	}

	switch key {
	case "claim_equals":
		typed, ok := value.(map[string]any)
		if !ok || len(typed) == 0 {
			return nil, fmt.Errorf("%w: claim_equals requires a non-empty object", ErrInvalidConfig)
		}

		matchers := make([]PresetMatcherFunc, 0, len(typed))
		for rawKey, expected := range typed {
			claimKey := rawKey
			expectedCopy := expected

			matchers = append(matchers, func(claims map[string]any) bool {
				claimValue, exists := claims[claimKey]
				if !exists {
					return false
				}

				switch exp := expectedCopy.(type) {
				case string:
					actualVal, ok := claimValue.(string)
					return ok && actualVal == exp
				case float64:
					actualVal, ok := claimValue.(float64)
					return ok && actualVal == exp
				case bool:
					actualVal, ok := claimValue.(bool)
					return ok && actualVal == exp
				case nil:
					return claimValue == nil
				default:
					return fmt.Sprintf("%v", claimValue) == fmt.Sprintf("%v", expectedCopy)
				}
			})
		}

		return MatcherAnd(matchers...), nil

	case "claim_exists":
		rawList, ok := value.([]any)
		if !ok || len(rawList) == 0 {
			return nil, fmt.Errorf("%w: claim_exists requires a non-empty array", ErrInvalidConfig)
		}

		claimNames := make([]string, len(rawList))
		for i, item := range rawList {
			name, ok := item.(string)
			if !ok || strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("%w: claim_exists array elements must be non-empty strings", ErrInvalidConfig)
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

	case "claim_contains":
		typed, ok := value.(map[string]any)
		if !ok || len(typed) == 0 {
			return nil, fmt.Errorf("%w: claim_contains requires a non-empty object", ErrInvalidConfig)
		}

		matchers := make([]PresetMatcherFunc, 0, len(typed))
		for rawKey, rawVal := range typed {
			valueStr, ok := rawVal.(string)
			if !ok {
				return nil, fmt.Errorf("%w: claim_contains values must be strings", ErrInvalidConfig)
			}
			claimKey := rawKey
			substring := valueStr
			matchers = append(matchers, ClaimContains(claimKey, substring))
		}

		return MatcherAnd(matchers...), nil

	case "has_scope":
		scope, ok := value.(string)
		if !ok || strings.TrimSpace(scope) == "" {
			return nil, fmt.Errorf("%w: has_scope requires a non-empty string", ErrInvalidConfig)
		}
		return HasScope(scope), nil

	case "has_any_scope":
		scopes, err := parseNonEmptyStringArray("has_any_scope", value)
		if err != nil {
			return nil, err
		}
		return HasAnyScope(scopes...), nil

	case "has_all_scopes":
		scopes, err := parseNonEmptyStringArray("has_all_scopes", value)
		if err != nil {
			return nil, err
		}
		return HasAllScopes(scopes...), nil

	case "cel_expression":
		expression, ok := value.(string)
		if !ok || strings.TrimSpace(expression) == "" {
			return nil, fmt.Errorf("%w: cel_expression requires a non-empty string", ErrInvalidConfig)
		}

		program, err := compileCELExpression(expression)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCEL, err)
		}

		if cache := getCELCache(); cache != nil {
			cache.Put(expression, program)
		}

		return func(claims map[string]any) bool {
			return evaluateCELProgram(program, claims)
		}, nil

	case "matcher_and", "matcher_or":
		rawList, ok := value.([]any)
		if !ok || len(rawList) == 0 {
			return nil, fmt.Errorf("%w: %s requires a non-empty array", ErrInvalidConfig, key)
		}

		matchers := make([]PresetMatcherFunc, 0, len(rawList))
		for idx, item := range rawList {
			condMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: %s[%d] must be an object", ErrInvalidConfig, key, idx)
			}

			matcher, err := conditionsToMatcher(condMap)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", key, idx, err)
			}
			matchers = append(matchers, matcher)
		}

		if key == "matcher_and" {
			return MatcherAnd(matchers...), nil
		}
		return MatcherOr(matchers...), nil
	}

	return nil, fmt.Errorf("%w: unknown condition type %q", ErrInvalidConfig, key)
}

func parseNonEmptyStringArray(key string, value any) ([]string, error) {
	rawList, ok := value.([]any)
	if !ok || len(rawList) == 0 {
		return nil, fmt.Errorf("%w: %s requires a non-empty array", ErrInvalidConfig, key)
	}

	out := make([]string, len(rawList))
	for i, item := range rawList {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("%w: %s array elements must be non-empty strings", ErrInvalidConfig, key)
		}
		out[i] = s
	}
	return out, nil
}

// validateValidationRulesConfig validates a ValidationRulesConfig including CEL rules.
func validateValidationRulesConfig(config *ValidationRulesConfig, path string) error {
	// Validate leeway
	if config.Leeway != "" {
		if _, err := time.ParseDuration(config.Leeway); err != nil {
			return fmt.Errorf("%w: invalid %s.leeway: %v", ErrInvalidDuration, path, err)
		}
	}

	// Validate token lifetime
	if config.TokenLifetime != nil {
		if config.TokenLifetime.Min != "" {
			if _, err := time.ParseDuration(config.TokenLifetime.Min); err != nil {
				return fmt.Errorf("%w: invalid %s.token_lifetime.min: %v", ErrInvalidDuration, path, err)
			}
		}

		if config.TokenLifetime.Max != "" {
			if _, err := time.ParseDuration(config.TokenLifetime.Max); err != nil {
				return fmt.Errorf("%w: invalid %s.token_lifetime.max: %v", ErrInvalidDuration, path, err)
			}
		}
	}

	// Validate CEL rules
	for i := range config.CELRules {
		rule := &config.CELRules[i]

		if rule.Name == "" {
			return fmt.Errorf("%w: %s.cel_rules[%d].name is required", ErrInvalidConfig, path, i)
		}

		if rule.Expression == "" {
			return fmt.Errorf("%w: %s.cel_rules[%d].expression is required", ErrInvalidConfig, path, i)
		}

		if _, err := compileCELExpression(rule.Expression); err != nil {
			return fmt.Errorf("%w: invalid %s.cel_rules[%d].expression: %v", ErrInvalidCEL, path, i, err)
		}
	}

	return nil
}
