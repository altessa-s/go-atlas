// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// newJWKSBodyIdP serves discovery plus a JWKS whose raw body is swappable.
func newJWKSBodyIdP(t *testing.T, initial string) (discoveryURL string, setJWKS func(string)) {
	t.Helper()
	var body atomic.Pointer[string]
	body.Store(&initial)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"jwks_uri":                              srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(*body.Load()))
	})
	return srv.URL + "/.well-known/openid-configuration", func(s string) { body.Store(&s) }
}

// jwksDoc renders a valid JWK Set with n distinct keys plus an ignored pad
// field of padLen bytes.
func jwksDoc(t *testing.T, n, padLen int) string {
	t.Helper()
	key := testhelpers.GenerateRSAKey(t, 2048)
	keys := make([]any, 0, n)
	for i := range n {
		keys = append(keys, rsaJWK(key, fmt.Sprintf("k%d", i)))
	}
	raw, err := json.Marshal(map[string]any{"keys": keys, "pad": strings.Repeat("a", padLen)})
	require.NoError(t, err)
	return string(raw)
}

func TestNewProvider_RejectsOversizedJWKS(t *testing.T) {
	t.Parallel()

	discoveryURL, _ := newJWKSBodyIdP(t, jwksDoc(t, 1, maxJWKSResponseSize))
	_, err := newRedirectTestProvider(t, discoveryURL)
	require.ErrorIs(t, err, ErrJWKSTooLarge)
}

func TestJWKSBodyBoundary(t *testing.T) {
	t.Parallel()
	doc := jwksDoc(t, 1, 0)
	for _, tc := range []struct {
		name     string
		bytes    int
		tooLarge bool
	}{
		{"exactly limit", maxJWKSResponseSize, false},
		{"one over including trailing whitespace", maxJWKSResponseSize + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			discoveryURL, _ := newJWKSBodyIdP(t, doc+strings.Repeat(" ", tc.bytes-len(doc)))
			_, err := newRedirectTestProvider(t, discoveryURL)
			if tc.tooLarge {
				require.ErrorIs(t, err, ErrJWKSTooLarge)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewProvider_RejectsTooManyJWKSKeys(t *testing.T) {
	t.Parallel()

	discoveryURL, _ := newJWKSBodyIdP(t, jwksDoc(t, maxJWKSKeys+1, 0))
	_, err := newRedirectTestProvider(t, discoveryURL)
	require.ErrorIs(t, err, ErrJWKSTooLarge)
}

func TestNewProvider_AcceptsJWKSAtKeyLimit(t *testing.T) {
	t.Parallel()

	discoveryURL, _ := newJWKSBodyIdP(t, jwksDoc(t, maxJWKSKeys, 0))
	_, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
}

func TestRefreshJWKS_RejectsOversizedDocumentAndKeepsKeys(t *testing.T) {
	t.Parallel()

	discoveryURL, setJWKS := newJWKSBodyIdP(t, jwksDoc(t, 1, 0))
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)

	original, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	freshness := p.lastJWKSRefreshUnixNanos.Load()
	for _, body := range []string{jwksDoc(t, 1, maxJWKSResponseSize), jwksDoc(t, maxJWKSKeys+1, 0)} {
		setJWKS(body)
		require.ErrorIs(t, p.RefreshJWKS(t.Context()), ErrJWKSTooLarge)
		keys, err := p.jwks.Storage().KeyReadAll(t.Context())
		require.NoError(t, err)
		require.Equal(t, original, keys)
		require.Equal(t, freshness, p.lastJWKSRefreshUnixNanos.Load())
	}
}

func TestJWKSRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	discoveryURL, _ := newJWKSBodyIdP(t, `{"keys":[]} {"keys":[]}`)
	_, err := newRedirectTestProvider(t, discoveryURL)
	var syntax *json.SyntaxError
	require.ErrorAs(t, err, &syntax)
}

func TestJWKSInvalidKeyPolicy(t *testing.T) {
	t.Parallel()
	discoveryURL, setJWKS := newJWKSBodyIdP(t, `{"keys":[{"kty":"RSA","n":"!","e":"AQAB"}]}`)
	_, err := newRedirectTestProvider(t, discoveryURL)
	var invalidBase64 base64.CorruptInputError
	require.ErrorAs(t, err, &invalidBase64)
	setJWKS(`{"keys":[{"kty":"future-key"}]}`)
	p, err := newRedirectTestProvider(t, discoveryURL)
	require.NoError(t, err)
	setJWKS(`{"keys":[{"kty":"RSA","n":"!","e":"AQAB"}]}`)
	require.NoError(t, p.RefreshJWKS(t.Context()))
	keys, err := p.jwks.Storage().KeyReadAll(t.Context())
	require.NoError(t, err)
	require.Empty(t, keys)
}
