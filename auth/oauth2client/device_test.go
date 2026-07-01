// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

func TestDeviceFlow(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "cli", r.Form.Get("client_id"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dev-code",
			"user_code":                 "WDJB-MJHT",
			"verification_uri":          "https://idp.example/device",
			"verification_uri_complete": "https://idp.example/device?user_code=WDJB-MJHT",
			"expires_in":                600,
			"interval":                  5,
		}))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", r.Form.Get("grant_type"))
		require.Equal(t, "dev-code", r.Form.Get("device_code"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "dev-token", "token_type": "Bearer", "expires_in": 3600,
		}))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	df := oauth2client.NewDeviceFlow(
		oauth2.Endpoint{DeviceAuthURL: srv.URL + "/device", TokenURL: srv.URL + "/token"},
		"cli", "", oauth2client.WithScopes("openid"),
	)

	da, err := df.Authorize(t.Context())
	require.NoError(t, err)
	require.Equal(t, "WDJB-MJHT", da.UserCode)
	require.Equal(t, "https://idp.example/device?user_code=WDJB-MJHT", da.VerificationURIComplete)

	src, tok, err := df.Token(t.Context(), da)
	require.NoError(t, err)
	require.Equal(t, "dev-token", tok.AccessToken)
	require.NotNil(t, src)
}

func TestDeviceFlowAuthorizeError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	df := oauth2client.NewDeviceFlow(
		oauth2.Endpoint{DeviceAuthURL: srv.URL, TokenURL: srv.URL}, "cli", "")
	_, err := df.Authorize(t.Context())
	require.ErrorIs(t, err, oauth2client.ErrDeviceAuth)
}
