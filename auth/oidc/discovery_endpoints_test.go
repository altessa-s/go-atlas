// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// loopbackProvider builds a Provider whose discovery URL is discoveryURL and
// whose discovery fetch returns body, in enforce mode.
func loopbackProvider(t *testing.T, discoveryURL, body string, allowHTTP bool) *Provider {
	t.Helper()
	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    r,
		}, nil
	})
	return &Provider{
		discoveryURL: discoveryURL,
		client:       &http.Client{Transport: rt},
		logger:       slog.New(slog.DiscardHandler),
		opts: &options{
			discoveryValidationMode:    DiscoveryValidationModeEnforce,
			discoveryAllowLoopbackHTTP: allowHTTP,
		},
	}
}

func TestDiscoveryEndpointGetters(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://issuer.example.com",
  "authorization_endpoint":"https://issuer.example.com/auth",
  "jwks_uri":"https://issuer.example.com/jwks",
  "end_session_endpoint":"https://issuer.example.com/logout",
  "revocation_endpoint":"https://issuer.example.com/revoke",
  "id_token_signing_alg_values_supported":["RS256"]
}`
	p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), body)
	require.NoError(t, p.getDiscoveryInfo(t.Context()))
	require.Equal(t, "https://issuer.example.com/auth", p.AuthorizationEndpoint())
	require.Equal(t, "https://issuer.example.com/logout", p.EndSessionEndpoint())
	require.Equal(t, "https://issuer.example.com/revoke", p.RevocationEndpoint())
}

func TestDiscoveryEndpointGettersBeforeDiscovery(t *testing.T) {
	t.Parallel()

	p := &Provider{}
	require.Empty(t, p.AuthorizationEndpoint())
	require.Empty(t, p.EndSessionEndpoint())
	require.Empty(t, p.RevocationEndpoint())
}

func TestDiscoveryEnforceValidatesNewEndpoints(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"end_session_endpoint", "revocation_endpoint"} {
		for name, value := range map[string]string{
			"cross_origin": "https://attacker.example.net/x",
			"plain_http":   "http://issuer.example.com/x",
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				t.Parallel()
				body := `{
  "issuer":"https://issuer.example.com",
  "jwks_uri":"https://issuer.example.com/jwks",
  "` + field + `":"` + value + `",
  "id_token_signing_alg_values_supported":["RS256"]
}`
				p := discoveryProvider(t, DiscoveryValidationModeEnforce, slog.New(slog.DiscardHandler), body)
				require.ErrorIs(t, p.getDiscoveryInfo(t.Context()), ErrDiscoveryValidation)
			})
		}
	}
}

func TestDiscoveryAllowLoopbackHTTP(t *testing.T) {
	t.Parallel()

	doc := func(issuer, jwks string) string {
		return `{"issuer":"` + issuer + `","jwks_uri":"` + jwks + `","id_token_signing_alg_values_supported":["RS256"]}`
	}
	tests := []struct {
		name      string
		issuer    string
		jwks      string
		allowHTTP bool
		wantErr   bool
	}{
		{"localhost", "http://localhost:8080", "http://localhost:8080/jwks", true, false},
		{"ipv4_loopback", "http://127.0.0.1:8080/realm", "http://127.0.0.1:8080/realm/jwks", true, false},
		{"ipv6_loopback", "http://[::1]:8080", "http://[::1]:8080/jwks", true, false},
		{"disabled_by_default", "http://localhost:8080", "http://localhost:8080/jwks", false, true},
		{"non_loopback_issuer", "http://issuer.example.com", "http://issuer.example.com/jwks", true, true},
		{"endpoint_other_loopback_port", "http://localhost:8080", "http://localhost:9090/jwks", true, true},
		{"endpoint_other_host", "http://localhost:8080", "http://evil.example.com/jwks", true, true},
		{"loopback_https_still_fine", "https://localhost:8443", "https://localhost:8443/jwks", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := loopbackProvider(t, tc.issuer+"/.well-known/openid-configuration", doc(tc.issuer, tc.jwks), tc.allowHTTP)
			err := p.getDiscoveryInfo(t.Context())
			if tc.wantErr {
				require.ErrorIs(t, err, ErrDiscoveryValidation)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "127.0.0.1": true, "127.9.9.9": true, "::1": true,
		"10.0.0.1": false, "localhost.example.com": false, "example.com": false, "": false,
	} {
		require.Equal(t, want, isLoopbackHost(host), host)
	}
}
