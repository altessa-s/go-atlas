// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package authconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"
)

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
	require.ErrorIs(t, err, rules.ErrInvalidConfig,
		"explicit verify_not_before: false must be rejected — the validator has no per-claim opt-out")
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
					VerifyNotBefore: &disabled,
				},
			},
		},
	}

	err := ValidateServiceConfig(cfg)
	require.ErrorIs(t, err, rules.ErrInvalidConfig)
	require.Contains(t, err.Error(), "presets[0].validation.verify_not_before",
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
			VerifyNotBefore: &enabled,
		},
	}

	require.NoError(t, ValidateServiceConfig(cfg))
}

func TestParseServiceConfig_RejectsFalseVerifyNotBeforeJSON(t *testing.T) {
	t.Parallel()

	// End-to-end via ParseServiceConfig to make sure the JSON unmarshal
	// path also surfaces the misconfiguration loudly.
	json := []byte(`{
		"version": "1.0",
		"default_validation": {
			"verify_not_before": false
		}
	}`)

	_, err := ParseServiceConfig(json)
	require.ErrorIs(t, err, rules.ErrInvalidConfig,
		"ParseServiceConfig must refuse verify_not_before: false at load time, not silently strip it")
}

func TestValidateServiceConfig_RejectsLeewayAboveMax(t *testing.T) {
	t.Parallel()

	cfg := &ServiceConfig{
		Version:           SupportedConfigVersion,
		DefaultValidation: &ValidationRulesConfig{Leeway: "24h"},
	}
	require.ErrorIs(t, ValidateServiceConfig(cfg), rules.ErrInvalidConfig)

	cfg.DefaultValidation.Leeway = "5m"
	require.NoError(t, ValidateServiceConfig(cfg))
}

func TestParseExactServiceConfig(t *testing.T) {
	t.Parallel()
	const config = `{"version":"1.0","default_validation":{"expected_claims":{"iat":1750000000.000000001}}}`
	cfg, err := ParseServiceConfig([]byte(config + " \r\n\t"))
	require.NoError(t, err)
	require.Equal(t, json.Number("1750000000.000000001"), cfg.DefaultValidation.ExpectedClaims["iat"])
	for _, suffix := range []string{"{}", "!"} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			_, err := ParseServiceConfig([]byte(config + suffix))
			require.ErrorIs(t, err, rules.ErrInvalidConfig)
		})
	}
}

func TestParseServiceConfigTokenLifetimeBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, policy string
		want         error
	}{
		{"negative minimum", `"default_validation":{"token_lifetime":{"min":"-1s"}}`, rules.ErrInvalidConfig},
		{"minimum above maximum", `"default_validation":{"token_lifetime":{"min":"2h","max":"1h"}}`, rules.ErrInvalidConfig},
		{"preset minimum above maximum", `"presets":[{"name":"selected","validation":{"token_lifetime":{"min":"2h","max":"1h"}}}]`, rules.ErrInvalidConfig},
		{"minimum equals maximum", `"default_validation":{"token_lifetime":{"min":"1h","max":"1h"}}`, nil},
		{"zero minimum", `"default_validation":{"token_lifetime":{"min":"0s","max":"1h"}}`, nil},
		{"zero maximum", `"default_validation":{"token_lifetime":{"min":"1h","max":"0s"}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseServiceConfig([]byte(`{"version":"1.0",` + tc.policy + `}`))
			require.ErrorIs(t, err, tc.want)
		})
	}
}
