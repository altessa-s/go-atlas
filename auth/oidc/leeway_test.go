// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

func TestNewProvider_RejectsLeewayAboveMax(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)

	_, err := idp.tryNewProvider(t, WithDefaultValidationOptions(WithValidationLeeway(jwt.MaxLeeway+time.Second)))
	require.ErrorIs(t, err, ErrInvalidConfig)

	_, err = idp.tryNewProvider(t, WithPresets(NewValidationPreset("lax", WithValidationLeeway(24*time.Hour))))
	require.ErrorIs(t, err, ErrInvalidConfig)

	_, err = idp.tryNewProvider(t, WithDefaultValidationOptions(WithValidationLeeway(jwt.MaxLeeway)))
	require.NoError(t, err)
}

func TestValidate_PerCallLeewayCapped(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t)

	expired := idp.sign(t, idp.claims(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}))
	_, err := p.ValidateTokenWithOptions(t.Context(), expired,
		WithValidationAudience(testAudience), WithValidationLeeway(24*time.Hour))
	require.ErrorIs(t, err, ErrTokenInvalid)
}

func TestValidateServiceConfig_RejectsLeewayAboveMax(t *testing.T) {
	t.Parallel()

	cfg := &ServiceConfig{
		Version:           SupportedConfigVersion,
		DefaultValidation: &ValidationRulesConfig{Leeway: "24h"},
	}
	require.ErrorIs(t, ValidateServiceConfig(cfg), ErrInvalidConfig)

	cfg.DefaultValidation.Leeway = "5m"
	require.NoError(t, ValidateServiceConfig(cfg))
}
