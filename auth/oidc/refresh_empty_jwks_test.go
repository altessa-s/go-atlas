// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshJWKS_KeepsKeysWhenDocumentHasNoUsableKey(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksDoc(t, 1, 0))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)

	original, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, original)
	freshness := p.lastJWKSRefreshUnixNanos.Load()

	for _, body := range []string{`{}`, `{"keys":[]}`, `{"keys":[{"kty":"RSA","n":"!","e":"AQAB"}]}`} {
		setJWKS(body)
		require.ErrorIs(t, p.RefreshJWKS(t.Context()), ErrDiscovery, body)

		keys, err := p.jwks.Storage().KeyReadAll(t.Context())
		require.NoError(t, err)
		require.Equal(t, original, keys, body)
		require.Equal(t, freshness, p.lastJWKSRefreshUnixNanos.Load(), body)
	}
}
