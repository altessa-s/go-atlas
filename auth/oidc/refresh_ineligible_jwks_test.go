// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// invalidEncJWK is an encryption key the JWK parser rejects: its modulus is
// not base64url. It could never verify a token.
func invalidEncJWK(kid string) map[string]any {
	key := corruptJWK(kid)
	key["use"] = "enc"
	return key
}

// An encryption key the parser rejects must not wedge rotation: it is never
// parsed, so the signing keys published next to it are rotated in, at startup
// and on every refresh.
func TestRefreshJWKS_InvalidEncryptionKeyDoesNotBlockRotation(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksOf(t, sigJWK(t, "old"), invalidEncJWK("enc")))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err, "the initial load ignores the encryption key too")
	require.Equal(t, []string{"old"}, storedKeyIDs(t, p))
	p.keys.lastJWKSRefresh.Store(nil)

	setJWKS(jwksOf(t, invalidEncJWK("enc"), sigJWK(t, "new1"), sigJWK(t, "new2")))
	require.NoError(t, p.RefreshJWKS(t.Context()))
	require.ElementsMatch(t, []string{"new1", "new2"}, storedKeyIDs(t, p))
	require.NotNil(t, p.keys.lastJWKSRefresh.Load(), "the rotation is fresh")
}

// The strict rule still covers every key that could verify a token: a
// malformed signing key, or one without "use", fails the refresh whatever
// else the document holds.
func TestRefreshJWKS_InvalidVerificationKeyStillFailsRefresh(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksOf(t, sigJWK(t, "old")))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)

	noUse := corruptJWK("no-use")
	delete(noUse, "use")
	for name, bad := range map[string]map[string]any{"sig": corruptJWK("bad"), "no use": noUse} {
		setJWKS(jwksOf(t, invalidEncJWK("enc"), bad, sigJWK(t, "new")))
		require.ErrorIs(t, p.RefreshJWKS(t.Context()), ErrDiscovery, name)
		require.Equal(t, []string{"old"}, storedKeyIDs(t, p), name)
	}
}
