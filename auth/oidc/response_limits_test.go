// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// jsonResponse wraps body into a 200 OK application/json response.
func jsonResponse(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

// The oversized bodies below are VALID JSON documents padded past the read
// cap with an ignored field: without the size limit both calls would succeed,
// so the tests pin the over-limit rejection itself, not a parse failure.

func TestProvider_getDiscoveryInfo_RejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	body := `{
  "issuer":"https://issuer",
  "authorization_endpoint":"https://issuer/auth",
  "token_endpoint":"https://issuer/token",
  "jwks_uri":"https://issuer/jwks",
  "userinfo_endpoint":"https://issuer/userinfo",
  "introspection_endpoint":"https://issuer/introspect",
  "id_token_signing_alg_values_supported":["RS256"],
  "pad":"` + strings.Repeat("a", maxDiscoveryResponseSize) + `"
}`

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(r, body), nil
	})

	p := &Provider{
		discoveryURL: "https://issuer/.well-known/openid-configuration",
		client:       &http.Client{Transport: rt},
		logger:       slog.New(slog.DiscardHandler),
	}

	err := p.getDiscoveryInfo(t.Context())
	require.ErrorIs(t, err, ErrDiscovery)
	require.Nil(t, p.discoveryInfo)
}

func TestProvider_IntrospectToken_RejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	body := `{"active":true,"pad":"` + strings.Repeat("a", maxIntrospectionResponseSize) + `"}`

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(r, body), nil
	})

	p := newRevocationTestProvider(t, rt, options{})

	resp, err := p.IntrospectToken(t.Context(), "any-token")
	require.ErrorIs(t, err, ErrIntrospection)
	require.Nil(t, resp)
}
