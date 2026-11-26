// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// Adjacent integers above 2^53 must stay distinct on a cold verification and on
// a signature-cache hit, where the entry is decoded from JSON again, whatever
// the type of the configured expectation.
func TestExpectedClaimsLargeIntegersColdAndWarm(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	cache := newMemCacher()
	p := idp.newProvider(t, WithTokenCache(cache))

	type expectation struct {
		name  string
		value func(n int64) any
	}
	expectations := []expectation{
		{"json.Number", func(n int64) any { return json.Number(strconv.FormatInt(n, 10)) }},
		{"int64", func(n int64) any { return n }},
		{"uint64", func(n int64) any { return uint64(n) }},
	}
	ids := []int64{9007199254740992, 9007199254740993}
	tokens := map[int64]string{}
	for _, id := range ids {
		tokens[id] = idp.sign(t, idp.claims(map[string]any{"tenant": json.Number(strconv.FormatInt(id, 10))}))
	}
	for pass := range 2 {
		for id, raw := range tokens {
			for _, other := range ids {
				for _, e := range expectations {
					_, err := p.ValidateTokenWithOptions(t.Context(), raw, WithValidationAudience(testAudience),
						WithValidationExpectedClaims(map[string]any{"tenant": e.value(other)}))
					if other == id {
						require.NoError(t, err, "pass %d %s token %d", pass, e.name, id)
					} else {
						require.ErrorIs(t, err, ErrTokenInvalid, "pass %d %s token %d matched %d", pass, e.name, id, other)
					}
				}
			}
		}
	}
	// One signature verification per token: every other validation was a cache hit.
	require.Len(t, cache.saves(), len(ids))
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
