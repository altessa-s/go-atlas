// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/auth/oauth2client/factory"
	"github.com/altessa-s/go-atlas/config"
)

// staticEndpoint is a test [oauth2client.TokenEndpointSource].
type staticEndpoint string

func (s staticEndpoint) TokenEndpoint() string { return string(s) }

func tokenServer(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form = r.Form
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "cc-token", "token_type": "Bearer", "expires_in": 3600,
		}))
	}))
	t.Cleanup(srv.Close)
	return srv, &form
}

func TestBuilderBuildFromTokenURL(t *testing.T) {
	t.Parallel()
	srv, form := tokenServer(t)

	cfg := &config.OAuth2Client{
		TokenUrl:  srv.URL,
		ClientId:  "svc",
		AuthStyle: "params",
		Scopes:    []string{"orders:read"},
	}
	src, err := factory.New(cfg).Build(t.Context())
	require.NoError(t, err)

	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "cc-token", tok.AccessToken)
	require.Equal(t, "client_credentials", form.Get("grant_type"))
	require.Equal(t, "orders:read", form.Get("scope"))
	require.Equal(t, "svc", form.Get("client_id")) // params auth style
}

func TestBuilderBuildFromDiscovery(t *testing.T) {
	t.Parallel()
	srv, _ := tokenServer(t)

	cfg := &config.OAuth2Client{
		DiscoveryUrl: "https://idp.example/.well-known/openid-configuration",
		ClientId:     "svc",
		AuthStyle:    "auto",
	}
	src, err := factory.New(cfg).
		UseTokenEndpointSource(staticEndpoint(srv.URL)).
		Build(t.Context())
	require.NoError(t, err)

	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "cc-token", tok.AccessToken)
}

func TestBuilderDiscoveryWithoutSource(t *testing.T) {
	t.Parallel()
	cfg := &config.OAuth2Client{DiscoveryUrl: "https://idp.example/x", ClientId: "svc", AuthStyle: "auto"}
	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err)
}

func TestBuilderNilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuilderBuildExchanger(t *testing.T) {
	t.Parallel()
	cfg := &config.OAuth2Client{
		TokenUrl:  "https://idp.example/token",
		ClientId:  "svc",
		AuthStyle: "auto",
		Retry:     &config.OAuth2ClientRetry{Attempts: 2},
	}
	ex, err := factory.New(cfg).BuildExchanger(t.Context())
	require.NoError(t, err)
	require.NotNil(t, ex)
	// A missing subject token is rejected regardless of transport.
	_, err = ex.Exchange(t.Context(), oauth2client.ExchangeRequest{})
	require.ErrorIs(t, err, oauth2client.ErrSubjectTokenRequired)
}
