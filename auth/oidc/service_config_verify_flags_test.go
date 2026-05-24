// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// applyValidationOptionsToVerifier replays a slice of ValidationOption onto
// a fresh verifierOptions so tests can assert which JWT-library knobs the
// service config actually flipped. Mirrors the runtime path in
// ValidateTokenWithOptions where the validator clones+applies the options.
func applyValidationOptionsToVerifier(opts []ValidationOption) *verifierOptions {
	v := &verifierOptions{}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func TestValidationRulesConfig_VerifyExpiration_NilOmitsRequirement(t *testing.T) {
	t.Parallel()

	cfg := &ValidationRulesConfig{} // VerifyExpiration nil
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	v := applyValidationOptionsToVerifier(opts)
	require.False(t, v.expirationRequired,
		"a missing verify_expiration field must NOT switch on the require-exp contract — preserves the JWT-library default")
}

func TestValidationRulesConfig_VerifyExpiration_TrueRequiresExp(t *testing.T) {
	t.Parallel()

	enabled := true
	cfg := &ValidationRulesConfig{VerifyExpiration: &enabled}
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	v := applyValidationOptionsToVerifier(opts)
	require.True(t, v.expirationRequired,
		"verify_expiration: true must propagate to WithValidationExpirationRequired so missing-exp tokens are rejected")
}

func TestValidationRulesConfig_VerifyNotBefore_NilOmitsRequirement(t *testing.T) {
	t.Parallel()

	cfg := &ValidationRulesConfig{} // VerifyNotBefore nil
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	v := applyValidationOptionsToVerifier(opts)
	require.False(t, v.notBeforeRequired)
}

func TestValidationRulesConfig_VerifyNotBefore_TrueRequiresNbf(t *testing.T) {
	t.Parallel()

	enabled := true
	cfg := &ValidationRulesConfig{VerifyNotBefore: &enabled}
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	v := applyValidationOptionsToVerifier(opts)
	require.True(t, v.notBeforeRequired,
		"verify_not_before: true must propagate to WithValidationNotBeforeRequired")
}

func TestValidateServiceConfig_RejectsExplicitFalseVerifyExpiration(t *testing.T) {
	t.Parallel()

	disabled := false
	cfg := &ServiceConfig{
		Version: SupportedConfigVersion,
		DefaultValidation: &ValidationRulesConfig{
			VerifyExpiration: &disabled,
		},
	}

	err := ValidateServiceConfig(cfg)
	require.ErrorIs(t, err, ErrInvalidConfig,
		"explicit verify_expiration: false must be rejected — the JWT library has no per-claim opt-out, so silently honoring it would mislead operators")
	require.Contains(t, err.Error(), "default_validation.verify_expiration",
		"error must point operators at the offending path")
}

func TestValidateServiceConfig_RejectsExplicitFalseVerifyNotBefore(t *testing.T) {
	t.Parallel()

	disabled := false
	cfg := &ServiceConfig{
		Version: SupportedConfigVersion,
		DefaultValidation: &ValidationRulesConfig{
			VerifyNotBefore: &disabled,
		},
	}

	err := ValidateServiceConfig(cfg)
	require.ErrorIs(t, err, ErrInvalidConfig,
		"explicit verify_not_before: false must be rejected for the same reason as verify_expiration: false")
	require.Contains(t, err.Error(), "default_validation.verify_not_before")
}

func TestValidateServiceConfig_RejectsFalseInsidePresetValidation(t *testing.T) {
	t.Parallel()

	// Regression: the check must also fire for nested validation blocks
	// inside presets, not just on default_validation. A misconfiguration
	// hidden in a preset is just as silent and just as wrong.
	disabled := false
	cfg := &ServiceConfig{
		Version: SupportedConfigVersion,
		Presets: []PresetDefinition{
			{
				Name: "service",
				Validation: &ValidationRulesConfig{
					VerifyExpiration: &disabled,
				},
			},
		},
	}

	err := ValidateServiceConfig(cfg)
	require.ErrorIs(t, err, ErrInvalidConfig)
	require.Contains(t, err.Error(), "presets[0].validation.verify_expiration",
		"error must locate the misconfiguration inside the preset, not just blame default_validation")
}

func TestValidateServiceConfig_AcceptsExplicitTrueVerifyFlags(t *testing.T) {
	t.Parallel()

	// Sanity: the only explicit-bool values we accept are nil (omit) and
	// true. true must round-trip through validation without error so
	// operators can adopt the stricter contract.
	enabled := true
	cfg := &ServiceConfig{
		Version: SupportedConfigVersion,
		DefaultValidation: &ValidationRulesConfig{
			VerifyExpiration: &enabled,
			VerifyNotBefore:  &enabled,
		},
	}

	require.NoError(t, ValidateServiceConfig(cfg))
}

func TestParseServiceConfig_RejectsFalseVerifyExpirationJSON(t *testing.T) {
	t.Parallel()

	// End-to-end via ParseServiceConfig to make sure the JSON unmarshal
	// path also surfaces the misconfiguration loudly.
	json := []byte(`{
		"version": "1.0",
		"default_validation": {
			"verify_expiration": false
		}
	}`)

	_, err := ParseServiceConfig(json)
	require.ErrorIs(t, err, ErrInvalidConfig,
		"ParseServiceConfig must refuse verify_expiration: false at load time, not silently strip it")
}
