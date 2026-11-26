// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Adjacent integers above 2^53 must stay distinct on a cold verification and on
// a signature-cache hit, where the entry is decoded from JSON again.
func TestExpectedClaimsLargeIntegersColdAndWarm(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithTokenCache(newMemCacher()))

	tokens := map[string]string{
		"9007199254740993": idp.sign(t, idp.claims(map[string]any{"tenant": json.Number("9007199254740993")})),
		"9007199254740992": idp.sign(t, idp.claims(map[string]any{"tenant": json.Number("9007199254740992")})),
	}
	for pass := range []string{"cold", "warm"} {
		for tenant, raw := range tokens {
			for other := range tokens {
				_, err := p.ValidateTokenWithOptions(t.Context(), raw, WithValidationAudience(testAudience),
					WithValidationExpectedClaims(map[string]any{"tenant": json.Number(other)}))
				if other == tenant {
					require.NoError(t, err, "pass %d tenant %s", pass, tenant)
				} else {
					require.ErrorIs(t, err, ErrTokenInvalid, "pass %d token %s matched %s", pass, tenant, other)
				}
			}
		}
	}
}

// Ordinary numbers keep their float64 type through a cache round trip.
func TestCachedNumbersKeepFloat64(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := idp.newProvider(t, WithTokenCache(newMemCacher()))
	raw := idp.sign(t, idp.claims(map[string]any{"level": 3}))

	for range 2 {
		claims, err := p.ValidateTokenWithOptions(t.Context(), raw, WithValidationAudience(testAudience),
			WithValidationExpectedClaims(map[string]any{"level": 3}))
		require.NoError(t, err)
		require.InDelta(t, 3, claims["level"], 0)
		_, isFloat := claims["level"].(float64)
		require.True(t, isFloat)
	}
}
