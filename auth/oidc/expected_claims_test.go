// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaimMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		actual   any
		expected any
		want     bool
	}{
		{name: "bool equal", actual: true, expected: true, want: true},
		{name: "bool differs", actual: false, expected: true, want: false},
		{name: "string true does not satisfy bool", actual: "true", expected: true, want: false},
		{name: "string equal", actual: "acme", expected: "acme", want: true},
		{name: "string differs", actual: "acme", expected: "evil", want: false},
		{name: "string expectation matches bool text", actual: true, expected: "true", want: true},
		{name: "string expectation rejects other bool", actual: false, expected: "true", want: false},
		{name: "number equal float", actual: float64(42), expected: 42, want: true},
		{name: "number equal json.Number", actual: json.Number("42"), expected: float64(42), want: true},
		{name: "number differs", actual: float64(41), expected: 42, want: false},
		{name: "string number does not satisfy number", actual: "42", expected: 42, want: false},
		{name: "string expectation matches number", actual: float64(42), expected: "42", want: true},
		{name: "null equal", actual: nil, expected: nil, want: true},
		{name: "null does not satisfy string", actual: nil, expected: "<nil>", want: false},
		{name: "array membership", actual: []any{"a", "b"}, expected: "b", want: true},
		{name: "array missing", actual: []any{"a", "b"}, expected: "c", want: false},
		{name: "array text is not a member", actual: []any{"a", "b"}, expected: "[a b]", want: false},
		{name: "string array membership", actual: []string{"a", "b"}, expected: "a", want: true},
		{name: "object never matches", actual: map[string]any{"a": "b"}, expected: "map[a:b]", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, claimMatches(tc.actual, tc.expected))
		})
	}
}

func TestValidate_ExpectedClaimsTyped(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t)

	stringTrue := idp.sign(t, idp.claims(map[string]any{"email_verified": "true"}))
	_, err := p.ValidateTokenWithOptions(t.Context(), stringTrue,
		WithValidationAudience(testAudience), WithValidationExpectedClaims(map[string]any{"email_verified": true}))
	require.ErrorIs(t, err, ErrTokenInvalid)

	boolTrue := idp.sign(t, idp.claims(map[string]any{"email_verified": true}))
	_, err = p.ValidateTokenWithOptions(t.Context(), boolTrue,
		WithValidationAudience(testAudience), WithValidationExpectedClaims(map[string]any{"email_verified": true}))
	require.NoError(t, err)
}

func TestValidationRulesConfig_ExpectedClaimsKeepTypes(t *testing.T) {
	t.Parallel()

	cfg := &ValidationRulesConfig{ExpectedClaims: map[string]any{"email_verified": true, "tier": float64(2)}}
	opts, err := cfg.ToValidationOptions()
	require.NoError(t, err)

	v := applyValidationOptionsToVerifier(opts)
	require.Equal(t, map[string]any{"email_verified": true, "tier": float64(2)}, v.expectedClaims)
}
