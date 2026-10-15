// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// rsaJWK renders key as a public RSA JWK under kid.
func rsaJWK(key *rsa.PrivateKey, kid string) map[string]any {
	return map[string]any{
		"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

// redirectIdP is an httptest issuer whose /jwks endpoint either serves its
// own key or answers with a redirect to a configurable location.
type redirectIdP struct {
	srv        *httptest.Server
	key        *rsa.PrivateKey
	redirectTo atomic.Pointer[string]
}

func newRedirectIdP(t *testing.T) *redirectIdP {
	t.Helper()
	idp := &redirectIdP{key: testhelpers.GenerateRSAKey(t, 2048)}
	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"jwks_uri":                              idp.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		if to := idp.redirectTo.Load(); to != nil {
			http.Redirect(w, r, *to, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(idp.key, "idp")}})
	})
	mux.HandleFunc("/jwks-moved", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(idp.key, "idp")}})
	})
	return idp
}

func (idp *redirectIdP) discoveryURL() string {
	return idp.srv.URL + "/.well-known/openid-configuration"
}

func (idp *redirectIdP) redirectJWKS(to string) { idp.redirectTo.Store(&to) }

// newAttackerJWKS serves a JWKS holding an attacker-controlled key.
func newAttackerJWKS(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	key := testhelpers.GenerateRSAKey(t, 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{rsaJWK(key, "attacker")}})
	}))
	t.Cleanup(srv.Close)
	return srv, key
}

func newRedirectTestProvider(t *testing.T, discoveryURL string, opts ...Option) (*Provider, error) {
	t.Helper()
	base := []Option{
		WithLogger(slog.New(slog.DiscardHandler)),
		WithHTTPClient(&http.Client{}),
		// httptest serves plain HTTP on loopback.
		WithDiscoveryAllowLoopbackHTTP(),
		WithDefaultValidationOptions(WithValidationAudience(testAudience)),
	}
	p, err := NewProvider(t.Context(), discoveryURL, append(base, opts...)...)
	if err == nil {
		t.Cleanup(p.Close)
	}
	return p, err
}

func signWith(t *testing.T, key *rsa.PrivateKey, kid, issuer string) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "sub": "user-1", "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	require.NoError(t, err)
	return raw
}

func TestNewProvider_RejectsCrossOriginJWKSRedirect(t *testing.T) {
	t.Parallel()

	idp := newRedirectIdP(t)
	attacker, _ := newAttackerJWKS(t)
	idp.redirectJWKS(attacker.URL + "/jwks")

	_, err := newRedirectTestProvider(t, idp.discoveryURL())
	require.ErrorIs(t, err, ErrRedirectNotAllowed)
}

func TestRefreshJWKS_RejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	idp := newRedirectIdP(t)
	attacker, attackerKey := newAttackerJWKS(t)

	p, err := newRedirectTestProvider(t, idp.discoveryURL())
	require.NoError(t, err)

	idp.redirectJWKS(attacker.URL + "/jwks")
	require.ErrorIs(t, p.RefreshJWKS(t.Context()), ErrRedirectNotAllowed)

	// The attacker key must not have been installed, the IdP key must remain.
	_, err = p.ValidateToken(t.Context(), signWith(t, attackerKey, "attacker", idp.srv.URL))
	require.Error(t, err)
	_, err = p.ValidateToken(t.Context(), signWith(t, idp.key, "idp", idp.srv.URL))
	require.NoError(t, err)
}

func TestRefreshJWKS_FollowsSameOriginRedirect(t *testing.T) {
	t.Parallel()

	idp := newRedirectIdP(t)
	p, err := newRedirectTestProvider(t, idp.discoveryURL())
	require.NoError(t, err)

	idp.redirectJWKS("/jwks-moved")
	require.NoError(t, p.RefreshJWKS(t.Context()))
}

func TestNewProvider_RejectsCrossOriginDiscoveryRedirect(t *testing.T) {
	t.Parallel()

	idp := newRedirectIdP(t)
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, idp.discoveryURL(), http.StatusFound)
	}))
	t.Cleanup(front.Close)

	// Warning mode must still reject unsafe redirects.
	_, err := newRedirectTestProvider(t, front.URL+"/.well-known/openid-configuration",
		WithDiscoveryValidationMode(DiscoveryValidationModeWarn))
	require.ErrorIs(t, err, ErrRedirectNotAllowed)
}

func TestNewProvider_DisabledValidationRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	idp := newRedirectIdP(t)
	attacker, _ := newAttackerJWKS(t)
	idp.redirectJWKS(attacker.URL + "/jwks")

	// Disabling discovery-document checks must not disable redirect protection.
	_, err := newRedirectTestProvider(t, idp.discoveryURL(),
		WithDiscoveryValidationMode(DiscoveryValidationModeDisabled))
	require.ErrorIs(t, err, ErrRedirectNotAllowed)
}

func TestProviderRedirectPolicy(t *testing.T) {
	t.Parallel()
	idp := newRedirectIdP(t)
	caller := &http.Client{}
	p, err := newRedirectTestProvider(t, idp.discoveryURL(), WithHTTPClient(caller))
	require.NoError(t, err)
	require.NotSame(t, caller, p.client)
	require.Nil(t, caller.CheckRedirect)
	require.NotNil(t, p.client.CheckRedirect)

	for _, tc := range []struct {
		name, from, to string
		allowed        bool
	}{
		{"same HTTPS origin", "https://issuer.example/start", "https://issuer.example/next", true},
		{"default port and case", "https://ISSUER.example:443/start", "https://issuer.example/next", true},
		{"changed host", "https://issuer.example/start", "https://attacker.example/next", false},
		{"changed port", "https://issuer.example/start", "https://issuer.example:444/next", false},
		{"downgrade", "https://issuer.example/start", "http://issuer.example/next", false},
		{"userinfo", "https://issuer.example/start", "https://user:secret@issuer.example/next", false},
		{"loopback opt in", "http://127.0.0.1:123/start", "http://127.0.0.1:123/next", true},
		{"loopback downgrade", "https://127.0.0.1:123/start", "http://127.0.0.1:123/next", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			from, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tc.from, nil)
			require.NoError(t, err)
			to, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tc.to, nil)
			require.NoError(t, err)
			err = p.client.CheckRedirect(to, []*http.Request{from})
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrRedirectNotAllowed)
			}
		})
	}
}

func TestProviderRedirectPreservesCallerPolicy(t *testing.T) {
	t.Parallel()
	idp := newRedirectIdP(t)
	want := errors.New("caller rejects redirect")
	caller := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return want }}
	p, err := newRedirectTestProvider(t, idp.discoveryURL(), WithHTTPClient(caller))
	require.NoError(t, err)
	idp.redirectJWKS("/jwks-moved")
	require.ErrorIs(t, p.RefreshJWKS(t.Context()), want)
}

func TestProviderRedirectWithAtlasClient(t *testing.T) {
	t.Parallel()
	idp := newRedirectIdP(t)
	attacker, _ := newAttackerJWKS(t)
	idp.redirectJWKS(attacker.URL + "/jwks")
	c := httpclient.New(httpclient.WithRetryMax(0), httpclient.WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")))
	_, err := newRedirectTestProvider(t, idp.discoveryURL(), WithHTTPClient(c))
	require.ErrorIs(t, err, ErrRedirectNotAllowed)
}

func TestProviderProtectsExplicitRevocationClient(t *testing.T) {
	t.Parallel()
	idp := newRedirectIdP(t)
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _, _ = w.Write([]byte("token\n")) }))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	t.Cleanup(source.Close)
	caller := &http.Client{}
	loader := &URLRevocationLoader{URL: source.URL, Client: caller}
	p, err := newRedirectTestProvider(t, idp.discoveryURL(), WithRevocationStorage(NewFilterRevocationStorage(noopFilter{}, loader, nil)), WithRevocationFailOpen())
	require.NoError(t, err)
	var got error
	protectedLoader := p.revocationStorage.(*filterRevocationStorage).loader.(*URLRevocationLoader)
	for _, err := range protectedLoader.StreamValues(t.Context()) {
		got = err
	}
	require.ErrorIs(t, got, ErrRedirectNotAllowed)
	require.Zero(t, hits.Load())
	require.NotSame(t, caller, protectedLoader.Client)
	require.Same(t, caller, loader.Client)
	require.Nil(t, caller.CheckRedirect)
}

func TestRedirectRechecksCallerMutation(t *testing.T) {
	t.Parallel()
	from, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example/start", nil)
	require.NoError(t, err)
	to := from.Clone(t.Context())
	c := protectedHTTPClient(&http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		req.URL.Host = "attacker.example"
		return nil
	}}, false)
	require.ErrorIs(t, c.CheckRedirect(to, []*http.Request{from}), ErrRedirectNotAllowed)
}

func TestRedirectPreservesUseLastResponse(t *testing.T) {
	t.Parallel()
	from, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example/start", nil)
	require.NoError(t, err)
	c := protectedHTTPClient(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}, false)
	require.ErrorIs(t, c.CheckRedirect(from.Clone(t.Context()), []*http.Request{from}), http.ErrUseLastResponse)
}

func TestProviderDoesNotMutateSharedRevocationLoader(t *testing.T) {
	t.Parallel()
	idp := newRedirectIdP(t)
	for _, client := range []*http.Client{nil, {}} {
		loader := &URLRevocationLoader{URL: idp.srv.URL, Client: client}
		storage := NewFilterRevocationStorage(noopFilter{}, loader, nil)
		a, err := newRedirectTestProvider(t, idp.discoveryURL(), WithRevocationStorage(storage), WithRevocationFailOpen())
		require.NoError(t, err)
		b, err := newRedirectTestProvider(t, idp.discoveryURL(), WithRevocationStorage(storage), WithRevocationFailOpen())
		require.NoError(t, err)
		require.Equal(t, client, loader.Client)
		require.NotSame(t, storage, a.revocationStorage)
		aLoader := a.revocationStorage.(*filterRevocationStorage).loader.(*URLRevocationLoader)
		bLoader := b.revocationStorage.(*filterRevocationStorage).loader.(*URLRevocationLoader)
		require.NotSame(t, loader, aLoader)
		require.NotSame(t, aLoader, bLoader)
		require.NotSame(t, aLoader.Client, bLoader.Client)
	}
}
