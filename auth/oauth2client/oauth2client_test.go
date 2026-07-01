// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

// tokenServer starts a token endpoint that records the last received form and
// replies with a bearer access token.
func tokenServer(t *testing.T, access string) (*httptest.Server, *url.Values) {
	t.Helper()
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": access,
			"token_type":   "Bearer",
			"expires_in":   3600,
		}))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotForm
}

func TestClientCredentials(t *testing.T) {
	t.Parallel()
	srv, form := tokenServer(t, "cc-token")

	src := oauth2client.ClientCredentials(t.Context(), srv.URL, "svc", "s3cr3t",
		oauth2client.WithScopes("orders:read", "orders:read", " orders:write "),
	)
	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "cc-token", tok.AccessToken)
	require.True(t, tok.Valid())

	require.Equal(t, "client_credentials", form.Get("grant_type"))
	// WithScopes trims and de-duplicates before joining.
	require.Equal(t, "orders:read orders:write", form.Get("scope"))
}

func TestRefresh(t *testing.T) {
	t.Parallel()
	srv, form := tokenServer(t, "refreshed-token")

	src := oauth2client.Refresh(t.Context(), srv.URL, "svc", "s3cr3t", "stored-refresh")
	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", tok.AccessToken)

	require.Equal(t, "refresh_token", form.Get("grant_type"))
	require.Equal(t, "stored-refresh", form.Get("refresh_token"))
}

func TestAuthCodeURL(t *testing.T) {
	t.Parallel()
	ac := oauth2client.NewAuthCode(
		oauth2.Endpoint{
			AuthURL:  "https://idp.example/authorize",
			TokenURL: "https://idp.example/token",
		},
		"svc", "s3cr3t", "https://app.example/callback",
		oauth2client.WithScopes("openid", "profile"),
	)

	raw := ac.AuthCodeURL("state-xyz")
	u, err := url.Parse(raw)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "svc", q.Get("client_id"))
	require.Equal(t, "https://app.example/callback", q.Get("redirect_uri"))
	require.Equal(t, "state-xyz", q.Get("state"))
	require.Equal(t, "openid profile", q.Get("scope"))
}

func TestAuthCodeExchange(t *testing.T) {
	t.Parallel()
	srv, form := tokenServer(t, "authcode-token")

	ac := oauth2client.NewAuthCode(
		oauth2.Endpoint{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL},
		"svc", "s3cr3t", "https://app.example/callback",
	)
	src, tok, err := ac.Exchange(t.Context(), "the-code")
	require.NoError(t, err)
	require.Equal(t, "authcode-token", tok.AccessToken)
	require.NotNil(t, src)

	require.Equal(t, "authorization_code", form.Get("grant_type"))
	require.Equal(t, "the-code", form.Get("code"))
}
