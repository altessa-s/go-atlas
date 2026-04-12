// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

// testContextKey is a custom type for context keys to avoid collisions (staticcheck SA1029).
type testContextKey string

func TestProvider_getDiscoveryInfo_UsesProvidedContext(t *testing.T) {
	ctx := context.WithValue(t.Context(), testContextKey("test"), "v")

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, ctx, r.Context(), "request context mismatch")

		body := `{
  "issuer":"https://issuer",
  "authorization_endpoint":"https://issuer/auth",
  "token_endpoint":"https://issuer/token",
  "jwks_uri":"https://issuer/jwks",
  "userinfo_endpoint":"https://issuer/userinfo",
  "introspection_endpoint":"https://issuer/introspect",
  "id_token_signing_alg_values_supported":["RS256"]
}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    r,
		}
		return resp, nil
	})

	p := &Provider{
		discoveryURL: "https://issuer/.well-known/openid-configuration",
		client:       &http.Client{Transport: rt},
		logger:       slog.New(slog.DiscardHandler),
	}

	require.NoError(t, p.getDiscoveryInfo(ctx))
	require.NotNil(t, p.discoveryInfo)
	require.True(t, p.discoveryInfo.IsValid(), "discoveryInfo invalid: %#v", p.discoveryInfo)
}

type staticTokenSource struct{ tok *oauth2.Token }

func (s staticTokenSource) Token() (*oauth2.Token, error) { return s.tok, nil }

func TestProvider_UserInfo_UsesProvidedContext(t *testing.T) {
	ctx := context.WithValue(t.Context(), testContextKey("test"), "v")

	rt := testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, ctx, r.Context(), "request context mismatch")
		require.Equal(t, "Bearer test", r.Header.Get("Authorization"))

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewBufferString(`{
  "sub":"u1",
  "email":"e@example.com"
}`)),
			Request: r,
		}
		return resp, nil
	})

	p := &Provider{
		client:        &http.Client{Transport: rt},
		discoveryInfo: &discoveryInfo{UserInfoURL: "https://issuer/userinfo"},
		logger:        slog.New(slog.DiscardHandler),
	}

	ui, err := p.UserInfo(ctx, staticTokenSource{tok: &oauth2.Token{AccessToken: "test", TokenType: "Bearer"}})
	require.NoError(t, err)
	require.NotNil(t, ui)
	require.Equal(t, "u1", ui.Id)
}
