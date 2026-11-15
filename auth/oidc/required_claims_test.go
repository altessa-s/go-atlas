// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidate_ExpAlwaysRequired pins that no validation option drops the
// exp requirement: custom required claims extend the mandatory set and
// ignored claims cannot remove exp.
func TestValidate_ExpAlwaysRequired(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t)

	tests := []struct {
		name string
		opts []ValidationOption
	}{
		{name: "custom required claims", opts: []ValidationOption{WithValidationRequiredClaims("sub", "email")}},
		{name: "exp ignored", opts: []ValidationOption{WithValidationIgnoredClaims("exp")}},
		{name: "defaults", opts: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			noExp := idp.sign(t, idp.claims(map[string]any{"exp": nil, "email": "a@example.com"}))
			opts := append([]ValidationOption{WithValidationAudience(testAudience)}, tc.opts...)
			_, err := p.ValidateTokenWithOptions(t.Context(), noExp, opts...)
			require.ErrorIs(t, err, ErrTokenInvalid)
		})
	}
}

// TestValidate_CustomRequiredClaimsExtendDefaults pins that a custom list
// adds to DefaultRequiredClaims instead of replacing it.
func TestValidate_CustomRequiredClaimsExtendDefaults(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t)

	noIat := idp.sign(t, idp.claims(map[string]any{"iat": nil, "email": "a@example.com"}))
	_, err := p.ValidateTokenWithOptions(t.Context(), noIat,
		WithValidationAudience(testAudience), WithValidationRequiredClaims("email"))
	require.ErrorIs(t, err, ErrTokenInvalid)

	noEmail := idp.sign(t, idp.claims(nil))
	_, err = p.ValidateTokenWithOptions(t.Context(), noEmail,
		WithValidationAudience(testAudience), WithValidationRequiredClaims("email"))
	require.ErrorIs(t, err, ErrTokenInvalid)

	valid := idp.sign(t, idp.claims(map[string]any{"email": "a@example.com"}))
	_, err = p.ValidateTokenWithOptions(t.Context(), valid,
		WithValidationAudience(testAudience), WithValidationRequiredClaims("email"))
	require.NoError(t, err)
}
