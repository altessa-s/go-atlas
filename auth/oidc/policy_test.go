// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestValidate_DefaultValidMethodsChangedAfterConstruction is deliberately
// serial: it mutates the exported DefaultValidMethods, which every test
// provider without explicit signing methods reads (parallel tests only start
// once all serial tests finished). The pre-compiled policies must keep
// honoring the current list, as a per-call verifier would.
func TestValidate_DefaultValidMethodsChangedAfterConstruction(t *testing.T) {
	idp := newTestIdP(t)
	cached := idp.sign(t, idp.claims(nil))

	noCache := idp.newProvider(t)
	withCache := idp.newProvider(t, WithTokenCache(newMemCacher()))
	autoPreset := idp.newProvider(t,
		WithPresets(NewValidationPreset("never", WithValidationAudience(testAudience))),
		WithPresetRules(PresetRule{PresetName: "never", Matcher: func(map[string]any) bool { return false }}))

	_, err := withCache.ValidateToken(t.Context(), cached)
	require.NoError(t, err, "precondition: the token is cached")

	saved := DefaultValidMethods
	DefaultValidMethods = []string{"ES256"}
	t.Cleanup(func() { DefaultValidMethods = saved })

	fresh := idp.sign(t, idp.claims(map[string]any{"jti": "fresh"}))
	for name, tc := range map[string]struct {
		p     *Provider
		token string
	}{
		"no cache":    {p: noCache, token: fresh},
		"cache miss":  {p: withCache, token: fresh},
		"cache hit":   {p: withCache, token: cached},
		"auto preset": {p: autoPreset, token: fresh},
	} {
		_, err := tc.p.ValidateToken(t.Context(), tc.token)
		require.ErrorIs(t, err, ErrTokenInvalid, name)
	}

	DefaultValidMethods = saved
	_, err = noCache.ValidateToken(t.Context(), fresh)
	require.NoError(t, err, "restoring the defaults reuses the pre-built verifier")
}

func TestCachedKeyFingerprint(t *testing.T) {
	t.Parallel()

	rsaA := &testhelpers.GenerateRSAKey(t, 2048).PublicKey
	rsaB := &testhelpers.GenerateRSAKey(t, 2048).PublicKey
	rsaACopy := &rsa.PublicKey{N: rsaA.N, E: rsaA.E}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	p := &Provider{}
	for _, key := range []any{rsaA, rsaA, rsaB, rsaACopy, rsaA, &ecKey.PublicKey, edPub, edPub, nil} {
		require.Equal(t, keyFingerprint(key), p.cachedKeyFingerprint(key))
	}
	require.Equal(t, keyFingerprint(rsaA), keyFingerprint(rsaACopy), "the fingerprint depends on the key content only")
	require.NotEqual(t, keyFingerprint(rsaA), keyFingerprint(rsaB))

	_ = p.cachedKeyFingerprint(edPub)
	require.Equal(t, &ecKey.PublicKey, p.lastKeyFingerprint.Load().key, "non-pointer keys bypass the memo")
}
