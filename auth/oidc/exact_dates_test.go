// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"
)

func TestExactCachedBoundaries(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	for _, tc := range []struct {
		claim, literal      string
		first, second       time.Duration
		firstErr, secondErr error
	}{
		{"nbf", "1750000000.75", 250 * time.Millisecond, 750 * time.Millisecond, ErrTokenInvalid, nil},
		{"exp", "1750000000.25", 200 * time.Millisecond, 300 * time.Millisecond, nil, ErrTokenInvalid},
		{"exp", "1750000001.000000001", time.Second, time.Second + time.Nanosecond, nil, ErrTokenInvalid},
		{"exp", "1750000001.0000000001", time.Second, time.Second + time.Nanosecond, nil, ErrTokenInvalid},
	} {
		t.Run(tc.claim+tc.literal, func(t *testing.T) {
			t.Parallel()
			cache := newMemCacher()
			p := idp.newProvider(t, WithTokenCache(cache), WithDefaultValidationOptions(WithValidationAudience(testAudience), WithValidationLeeway(0)))
			fresh := idp.newProvider(t, WithDefaultValidationOptions(WithValidationAudience(testAudience), WithValidationLeeway(0)))
			claims := idp.claims(map[string]any{"iat": 1750000000, "exp": 1750000100})
			claims[tc.claim] = json.Number(tc.literal)
			raw := idp.sign(t, claims)
			synctest.Test(t, func(t *testing.T) {
				start := time.Unix(1750000000, 0)
				// Cache the verified signature while at least one full cache tick remains.
				time.Sleep(start.Sub(time.Now()))
				_, _, err := p.policy.verifyTokenSignature(t.Context(), raw, p.policy.defaultPolicy)
				require.NoError(t, err)
				time.Sleep(tc.first)
				check := func(want error) {
					t.Helper()
					_, err := fresh.ValidateToken(t.Context(), raw)
					require.ErrorIs(t, err, want)
					got, err := p.ValidateToken(t.Context(), raw)
					require.ErrorIs(t, err, want)
					if err == nil {
						require.Equal(t, json.Number(tc.literal), got[tc.claim])
					}
					require.Len(t, cache.saves(), 1)
				}
				check(tc.firstErr)
				time.Sleep(tc.second - tc.first)
				check(tc.secondErr)
			})
		})
	}
}

func TestLegacySignatureCacheBypassed(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	cache := newMemCacher()
	p := idp.newProvider(t, WithTokenCache(cache))
	claims := idp.claims(map[string]any{"iat": json.Number("1.750000000000000001e9")})
	raw := idp.sign(t, claims)
	header := map[string]any{"alg": "RS256", "kid": idp.kid}
	legacy := tokenCacheKey(p.policy.tokensCacheKeyPrefix, "oidc-sig-v2\x00"+p.discovery.info.Issuer+"\x00"+p.discovery.info.JwksURL+"\x00"+raw)
	v3 := tokenCacheKey(p.policy.tokensCacheKeyPrefix, "oidc-sig-v3\x00"+p.discovery.info.Issuer+"\x00"+p.discovery.info.JwksURL+"\x00"+raw)
	claims["iat"] = float64(1750000000)
	require.NoError(t, cache.Save(t.Context(), legacy, cachedToken{Claims: claims, Header: header, KeyFingerprint: p.keys.currentKeyFingerprint(t.Context(), header)}, time.Hour))
	for range 2 {
		got, err := p.ValidateToken(t.Context(), raw)
		require.NoError(t, err)
		require.Equal(t, json.Number("1.750000000000000001e9"), got["iat"])
	}
	require.Contains(t, cache.items, v3)
	require.Len(t, cache.saves(), 2)
}

func TestExactLifetime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		iat, exp any
		max      time.Duration
		want     error
	}{
		{"fraction", json.Number("1750000000.1"), json.Number("1750000001.9"), time.Second, ErrTokenInvalid},
		{"reversed", json.Number("1750000000.9"), json.Number("1750000000.1"), time.Second, ErrTokenInvalid},
		{"sub-ns over", json.Number("1750000000"), json.Number("1750000001.0000000001"), time.Second, ErrTokenInvalid},
		{"equal residual", json.Number("1750000000.0000000001"), json.Number("1750000001.0000000001"), time.Second, nil},
		{"below duration ceiling", int64(0), int64(9223372036), math.MaxInt64, nil},
		{"above duration ceiling", int64(0), int64(9223372037), math.MaxInt64, ErrTokenInvalid},
		{"signed span", int64(math.MinInt64), int64(math.MaxInt64), math.MaxInt64, ErrTokenInvalid},
		{"signed span wrapping to zero", int64(math.MinInt64), int64(0), time.Second, ErrTokenInvalid},
		{"positive duration wrap", int64(0), int64(18446744074), time.Second, ErrTokenInvalid},
		{"reversed float", 1750000000.75, 1750000000.25, time.Second, ErrTokenInvalid},
		{"minimum", json.Number("-9223372036854775808"), json.Number("-9223372036854775807"), time.Second, nil},
		{"maximum", json.Number("9223372036854775806"), json.Number("9223372036854775807"), time.Second, nil},
		{"float below maximum", math.Nextafter(float64(1<<63), 0), math.Nextafter(float64(1<<63), 0), time.Second, nil},
		{"float beyond maximum", float64(1 << 63), float64(1 << 63), time.Second, ErrTokenInvalid},
		{"decimal beyond maximum", json.Number("9223372036854775807.01"), json.Number("9223372036854775807.01"), time.Second, ErrTokenInvalid},
		{"missing iat", nil, 1, time.Second, ErrTokenInvalid}, {"missing exp", 1, nil, time.Second, ErrTokenInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateTokenLifetime(map[string]any{"iat": tc.iat, "exp": tc.exp}, 0, tc.max), tc.want)
		})
	}
}

func TestExactCacheTTL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		exp  any
		want time.Duration
	}{
		{"expired", json.Number("1750000000.2499999999"), 0},
		{"below one tick", json.Number("1750000000.2500000001"), 0},
		{"one whole tick", json.Number("1750000000.2500000011"), time.Nanosecond},
		{"fractional now", json.Number("1750000002"), 1750 * time.Millisecond},
		{"saturated", int64(math.MaxInt64), math.MaxInt64},
		{"minimum", int64(math.MinInt64), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				time.Sleep(time.Unix(1750000000, 250000000).Sub(time.Now()))
				require.Equal(t, tc.want, getTokenExpirationTTL(map[string]any{"exp": tc.exp}))
			})
		})
	}
}

func TestExactCachedLifetime(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	for _, tc := range []struct {
		exp  string
		want error
	}{
		{"1750000001", nil}, {"1750000001.000000001", ErrTokenInvalid}, {"1750000001.0000000001", ErrTokenInvalid},
	} {
		t.Run(tc.exp, func(t *testing.T) {
			t.Parallel()
			cache := newMemCacher()
			p := idp.newProvider(t, WithTokenCache(cache), WithDefaultValidationOptions(WithValidationAudience(testAudience), WithValidationMaxTokenLifetime(time.Second)))
			raw := idp.sign(t, idp.claims(map[string]any{"iat": 1750000000, "exp": json.Number(tc.exp)}))
			synctest.Test(t, func(t *testing.T) {
				time.Sleep(time.Unix(1750000000, 500000000).Sub(time.Now()))
				for range 2 {
					_, err := p.ValidateToken(t.Context(), raw)
					require.ErrorIs(t, err, tc.want)
				}
				require.Len(t, cache.saves(), 1)
			})
		})
	}
}

func TestExactMinimumLifetime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		iat, exp any
		min, max time.Duration
		want     error
	}{
		{"equal", json.Number("1750000000.0000000001"), json.Number("1750000001.0000000001"), time.Second, 0, nil},
		{"nanosecond below", json.Number("1750000000"), json.Number("1750000000.999999999"), time.Second, 0, ErrTokenInvalid},
		{"sub-nanosecond below", json.Number("1750000000.0000000001"), json.Number("1750000001"), time.Second, 0, ErrTokenInvalid},
		{"fractional below", json.Number("1750000000.9"), json.Number("1750000001.1"), 250 * time.Millisecond, 0, ErrTokenInvalid},
		{"both bounds equal", json.Number("1750000000.9"), json.Number("1750000001.15"), 250 * time.Millisecond, 250 * time.Millisecond, nil},
		{"both bounds below", json.Number("1750000000.9"), json.Number("1750000001.1"), 250 * time.Millisecond, time.Second, ErrTokenInvalid},
		{"both bounds above", json.Number("1750000000.1"), json.Number("1750000001.9"), 250 * time.Millisecond, time.Second, ErrTokenInvalid},
		{"missing iat", nil, 1, time.Second, 0, ErrTokenInvalid},
		{"missing exp", 1, nil, time.Second, 0, ErrTokenInvalid},
		{"disabled", nil, nil, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateTokenLifetime(map[string]any{"iat": tc.iat, "exp": tc.exp}, tc.min, tc.max), tc.want)
		})
	}
}

func TestProviderRejectsInvalidLifetimeOptions(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	for _, tc := range []struct {
		name string
		opts []ValidationOption
	}{
		{"negative minimum", []ValidationOption{WithValidationMinTokenLifetime(-time.Second)}},
		{"minimum above maximum", []ValidationOption{WithValidationMinTokenLifetime(2 * time.Second), WithValidationMaxTokenLifetime(time.Second)}},
	} {
		t.Run(tc.name+"/default", func(t *testing.T) {
			t.Parallel()
			_, err := idp.tryNewProvider(t, WithDefaultValidationOptions(tc.opts...))
			require.ErrorIs(t, err, rules.ErrInvalidConfig)
		})
		t.Run(tc.name+"/preset", func(t *testing.T) {
			t.Parallel()
			_, err := idp.tryNewProvider(t, WithPresets(NewValidationPreset("test", tc.opts...)))
			require.ErrorIs(t, err, rules.ErrInvalidConfig)
		})
		t.Run(tc.name+"/per call", func(t *testing.T) {
			t.Parallel()
			p := idp.newProvider(t)
			_, err := p.ValidateTokenWithOptions(t.Context(), idp.sign(t, idp.claims(nil)), tc.opts...)
			require.ErrorIs(t, err, rules.ErrInvalidConfig)
		})
	}
}

func TestProviderMinimumLifetimeOverride(t *testing.T) {
	t.Parallel()
	idp := newTestIdP(t)
	p := idp.newProvider(t, WithDefaultValidationOptions(WithValidationAudience(testAudience), WithValidationMinTokenLifetime(2*time.Hour)))
	now := time.Now()
	token := idp.sign(t, idp.claims(map[string]any{"iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}))
	_, err := p.ValidateToken(t.Context(), token)
	require.ErrorIs(t, err, ErrTokenInvalid)
	_, err = p.ValidateTokenWithOptions(t.Context(), token, WithValidationMinTokenLifetime(0))
	require.NoError(t, err)
}
