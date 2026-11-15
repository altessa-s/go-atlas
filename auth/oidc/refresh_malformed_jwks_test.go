// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// jwksOf renders a JWK Set from raw key objects.
func jwksOf(t *testing.T, keys ...map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"keys": keys})
	require.NoError(t, err)
	return string(raw)
}

func sigJWK(t *testing.T, kid string) map[string]any {
	t.Helper()
	return rsaJWK(testhelpers.GenerateRSAKey(t, 2048), kid)
}

// corruptJWK is a signing key whose modulus is not base64url.
func corruptJWK(kid string) map[string]any {
	return map[string]any{"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig", "n": "!", "e": "AQAB"}
}

// A glitch that corrupts the active signing keys of a rotation and leaves one
// standby key readable must not evict the active keys: the refresh fails as a
// whole, like the initial load does on such a document, and the cached keys
// and their freshness stay.
func TestRefreshJWKS_MalformedKeyKeepsWorkingSet(t *testing.T) {
	t.Parallel()

	active := []map[string]any{sigJWK(t, "a1"), sigJWK(t, "a2"), sigJWK(t, "a3")}
	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksOf(t, active...))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
	original, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.Len(t, original, 3)
	freshness := p.lastJWKSRefreshUnixNanos.Load()

	for name, body := range map[string]string{
		"active keys corrupted, standby valid": jwksOf(t, corruptJWK("a1"), corruptJWK("a2"), corruptJWK("a3"), sigJWK(t, "standby")),
		"one corrupted among valid":            jwksOf(t, active[0], corruptJWK("a2"), active[2]),
	} {
		setJWKS(body)
		err = p.RefreshJWKS(t.Context())
		require.ErrorIs(t, err, ErrDiscovery, name)
		var invalidBase64 base64.CorruptInputError
		require.ErrorAs(t, err, &invalidBase64, name)

		keys, readErr := p.jwks.Storage().KeyReadAll(t.Context())
		require.NoError(t, readErr)
		require.Equal(t, original, keys, name)
		require.Equal(t, freshness, p.lastJWKSRefreshUnixNanos.Load(), name)
	}

	// The same document is refused at startup too.
	_, err = newRedirectTestProvider(t, discoveryURL)
	require.Error(t, err)
}

// A key of a type this build does not support is skipped by the refresh as it
// is by the initial load; the rest of the document is a rotation.
func TestRefreshJWKS_UnsupportedKeyTypeIsSkipped(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksOf(t, sigJWK(t, "old")))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)

	setJWKS(jwksOf(t, map[string]any{"kty": "future-key", "kid": "f"}, sigJWK(t, "new")))
	require.NoError(t, p.RefreshJWKS(t.Context()))
	require.Equal(t, []string{"new"}, storedKeyIDs(t, p))
}

// The initial load and a refresh unmarshal keys with the same options: a
// document holding a symmetric key yields the same key set on both paths.
func TestRefreshJWKS_SameKeysAsInitialLoad(t *testing.T) {
	t.Parallel()

	doc := jwksOf(t, sigJWK(t, "rsa"), map[string]any{"kty": "oct", "kid": "sym", "k": "c2VjcmV0LXNlY3JldC1zZWNyZXQ"})
	discoveryURL, _ := newJWKSBodyIdP(t, doc)
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
	initial, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.Len(t, initial, 2)

	require.NoError(t, p.RefreshJWKS(t.Context()))
	refreshed, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.Equal(t, initial, refreshed)
}

// idleCountingTransport counts CloseIdleConnections calls.
type idleCountingTransport struct {
	http.RoundTripper
	closed atomic.Int32
}

func (t *idleCountingTransport) CloseIdleConnections() { t.closed.Add(1) }

// Close must not close the idle connections of a client the caller supplied
// and may share with other components; it does for a client handed over with
// WithOwnedHTTPClient.
func TestProviderClose_ClosesIdleConnectionsOnlyOfOwnedClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       []Option
		wantClosed int32
	}{
		{name: "caller's client is left alone"},
		{name: "owned client is closed", opts: []Option{WithOwnedHTTPClient()}, wantClosed: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			transport, opts := callerClientOptions(t)

			p, err := NewProvider(t.Context(), idp.discoveryURL(), append(opts, tc.opts...)...)
			require.NoError(t, err)
			p.Close()
			require.Equal(t, tc.wantClosed, transport.closed.Load())
		})
	}

	t.Run("failed construction", func(t *testing.T) {
		t.Parallel()

		idp := newTestIdP(t)
		transport, opts := callerClientOptions(t)

		_, err := NewProvider(t.Context(), idp.discoveryURL(), append(opts, WithJWKSRefreshSchedule("not a cron"))...)
		require.Error(t, err)
		require.Zero(t, transport.closed.Load())
	})
}

// callerClientOptions returns provider options around an HTTP client the
// caller keeps, and the transport that counts its idle-connection closes.
func callerClientOptions(t *testing.T) (*idleCountingTransport, []Option) {
	t.Helper()
	base, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	inner := base.Clone()
	t.Cleanup(inner.CloseIdleConnections)
	transport := &idleCountingTransport{RoundTripper: inner}
	return transport, []Option{
		WithHTTPClient(&http.Client{Transport: transport}),
		// httptest serves plain HTTP on loopback.
		WithDiscoveryValidationMode(DiscoveryValidationModeDisabled),
		WithDefaultValidationOptions(WithValidationAudience(testAudience)),
	}
}
