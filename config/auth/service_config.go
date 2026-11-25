// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"

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
	//     WithValidationNotBeforeRequired at runtime.
	//   - false: REJECTED at config-load time with [rules.ErrInvalidConfig], so a
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
	// Min is the minimum acceptable exp - iat. An empty or zero duration disables it.
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
	// ErrInvalidVersion indicates unsupported configuration version.
	ErrInvalidVersion = errors.New("unsupported configuration version")

	// ErrInvalidDuration indicates invalid duration string format.
	ErrInvalidDuration = errors.New("invalid duration format")

	// ErrPresetNotFound indicates a referenced preset does not exist.
	ErrPresetNotFound = errors.New("preset not found")
)

// ParseServiceConfig parses and validates a ServiceConfig from JSON bytes.
//
// Example:
//
//	config, _ := authconfig.ParseServiceConfig(jsonData)
func ParseServiceConfig(data []byte) (*ServiceConfig, error) {
	var config ServiceConfig

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return nil, coreerrs.Wrapf(rules.ErrInvalidConfig, "failed to parse JSON: %v", err)
	}

	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, coreerrs.Wrap(rules.ErrInvalidConfig, "trailing data after service configuration")
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
		return coreerrs.Wrap(rules.ErrInvalidConfig, "version is required")
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
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "presets[%d].name is required", i)
		}

		if presetNames[preset.Name] {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "duplicate preset name %q", preset.Name)
		}
		presetNames[preset.Name] = true

		if preset.Validation == nil {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "presets[%d].validation is required", i)
		}

		if err := validateValidationRulesConfig(preset.Validation, fmt.Sprintf("presets[%d].validation", i)); err != nil {
			return err
		}
	}

	// Validate preset rules
	for i := range config.PresetRules {
		rule := &config.PresetRules[i]

		if rule.Preset == "" {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "preset_rules[%d].preset is required", i)
		}

		if !presetNames[rule.Preset] {
			return coreerrs.Wrapf(ErrPresetNotFound, "preset_rules[%d] references unknown preset %q",
				i, rule.Preset)
		}

		if len(rule.Conditions) == 0 {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "preset_rules[%d].conditions is required", i)
		}

		if _, err := rule.ParseConditions(); err != nil {
			return coreerrs.Wrapf(err, "preset_rules[%d].conditions", i)
		}
	}

	return nil
}

// validateValidationRulesConfig validates a ValidationRulesConfig including CEL rules.
func validateValidationRulesConfig(config *ValidationRulesConfig, path string) error {
	// Validate leeway
	if config.Leeway != "" {
		leeway, err := time.ParseDuration(config.Leeway)
		if err != nil {
			return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.leeway: %v", path, err)
		}
		if err := rules.CheckLeeway(leeway); err != nil {
			return coreerrs.Wrapf(err, "%s.leeway", path)
		}
	}

	// Reject an explicit `false` on the verify_not_before switch. The
	// validator has no per-claim opt-out, so silently honoring `false` here
	// would leave operators thinking they disabled a check that is still
	// running. Refusing the value at config-load time keeps the
	// misconfiguration loud.
	if config.VerifyNotBefore != nil && !*config.VerifyNotBefore {
		return coreerrs.Wrapf(rules.ErrInvalidConfig,
			"%s.verify_not_before: explicit `false` is not supported — omit the field to keep the JWT default or set it to true to require the nbf claim",
			path)
	}

	// Validate token lifetime
	if config.TokenLifetime != nil {
		var minLifetime, maxLifetime time.Duration
		var err error
		if config.TokenLifetime.Min != "" {
			minLifetime, err = time.ParseDuration(config.TokenLifetime.Min)
			if err != nil {
				return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.token_lifetime.min: %v", path, err)
			}
		}

		if config.TokenLifetime.Max != "" {
			maxLifetime, err = time.ParseDuration(config.TokenLifetime.Max)
			if err != nil {
				return coreerrs.Wrapf(ErrInvalidDuration, "invalid %s.token_lifetime.max: %v", path, err)
			}
		}

		if err := rules.CheckTokenLifetime(minLifetime, maxLifetime); err != nil {
			return coreerrs.Wrapf(err, "%s.token_lifetime", path)
		}
	}

	// Validate CEL rules
	for i := range config.CELRules {
		rule := &config.CELRules[i]

		if rule.Name == "" {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "%s.cel_rules[%d].name is required", path, i)
		}

		if rule.Expression == "" {
			return coreerrs.Wrapf(rules.ErrInvalidConfig, "%s.cel_rules[%d].expression is required", path, i)
		}

		if err := rules.ValidateCEL(rule.Expression); err != nil {
			return coreerrs.Wrapf(rules.ErrInvalidCEL, "invalid %s.cel_rules[%d].expression: %v", path, i, err)
		}
	}

	return nil
}
