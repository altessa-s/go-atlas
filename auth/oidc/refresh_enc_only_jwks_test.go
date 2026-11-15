// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/json"
	"testing"

	"github.com/MicahParks/jwkset"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// jwksWithUses renders a JWK Set with one valid RSA key per entry of uses,
// named k0, k1, …; an empty use omits the "use" member.
func jwksWithUses(t *testing.T, uses ...string) string {
	t.Helper()
	keys := make([]any, 0, len(uses))
	for i, use := range uses {
		jwk := rsaJWK(testhelpers.GenerateRSAKey(t, 2048), "k"+string(rune('0'+i)))
		if use == "" {
			delete(jwk, "use")
		} else {
			jwk["use"] = use
		}
		keys = append(keys, jwk)
	}
	raw, err := json.Marshal(map[string]any{"keys": keys})
	require.NoError(t, err)
	return string(raw)
}

func storedKeyIDs(t *testing.T, p *Provider) []string {
	t.Helper()
	keys, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, key.Marshal().KID)
	}
	return ids
}

// The refresh guard and the keyfunc whitelist go by the same predicate.
func TestVerificationEligible_MatchesKeyfuncWhitelist(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []jwkset.USE{jwkset.UseSig, ""}, jwksVerificationUses)
	for use, want := range map[string]bool{"sig": true, "": true, "enc": false} {
		var set struct {
			Keys []jwkset.JWKMarshal `json:"keys"`
		}
		require.NoError(t, json.Unmarshal([]byte(jwksWithUses(t, use)), &set))
		require.Equal(t, want, verificationEligible(set.Keys[0]), "use %q", use)
	}
}

// A document holding only keys that cannot verify a token — the "enc" keys of
// a half-published rotation — must not replace a working key set: every token
// would be rejected until the next refresh.
func TestRefreshJWKS_KeepsKeysWhenDocumentHasOnlyEncKeys(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksDoc(t, 1, 0))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
	original, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, original)
	freshness := p.lastJWKSRefreshUnixNanos.Load()

	for _, body := range []string{jwksWithUses(t, "enc"), jwksWithUses(t, "enc", "enc")} {
		setJWKS(body)
		require.ErrorIs(t, p.RefreshJWKS(t.Context()), ErrDiscovery)

		keys, err := p.jwks.Storage().KeyReadAll(t.Context())
		require.NoError(t, err)
		require.Equal(t, original, keys)
		require.Equal(t, freshness, p.lastJWKSRefreshUnixNanos.Load(), "a rejected document is not fresh")
	}
}

// With no key in place there is nothing to protect: the refresh succeeds, as
// with any other document without a usable key, and leaves the set empty —
// keys that cannot verify a token are not stored.
func TestRefreshJWKS_EncOnlyDocumentOnEmptySetIsAccepted(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, `{"keys":[{"kty":"future-key"}]}`)
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
	require.Empty(t, storedKeyIDs(t, p))

	setJWKS(jwksWithUses(t, "enc"))
	require.NoError(t, p.RefreshJWKS(t.Context()))
	require.Empty(t, storedKeyIDs(t, p))
}

// A document with at least one verification key is a rotation, whatever else
// it carries.
func TestRefreshJWKS_ReplacesKeysWhenDocumentHasVerificationKey(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		uses []string
		want []string
	}{
		"sig and enc":         {uses: []string{"sig", "enc"}, want: []string{"k0"}},
		"enc and no use":      {uses: []string{"enc", ""}, want: []string{"k1"}},
		"enc first, sig last": {uses: []string{"enc", "enc", "sig"}, want: []string{"k2"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksWithUses(t, "sig", "sig", "sig", "sig"))
			p, err := newRedirectTestProvider(t, discoveryURL)
			require.NoError(t, err)
			p.lastJWKSRefreshUnixNanos.Store(1)

			setJWKS(jwksWithUses(t, tc.uses...))
			require.NoError(t, p.RefreshJWKS(t.Context()))
			require.Equal(t, tc.want, storedKeyIDs(t, p), "only verification keys are stored")
			require.Greater(t, p.lastJWKSRefreshUnixNanos.Load(), int64(1), "a rotation is fresh")
		})
	}
}
