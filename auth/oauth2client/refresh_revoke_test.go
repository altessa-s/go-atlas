// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
)

func TestClientCredentialsEarlyExpiry(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "cc", "token_type": "Bearer", "expires_in": 100,
		}))
	}))
	t.Cleanup(srv.Close)

	// An early-expiry window far larger than the token lifetime makes every
	// Token() call treat the cached token as expired and refetch.
	src := oauth2client.ClientCredentials(t.Context(), srv.URL, "svc", "sec",
		oauth2client.WithEarlyExpiry(time.Hour))
	_, err := src.Token()
	require.NoError(t, err)
	_, err = src.Token()
	require.NoError(t, err)
	require.Equal(t, int32(2), hits.Load())
}

// revocationServer records the last form and Authorization header.
func revocationServer(t *testing.T, status int) (*httptest.Server, *url.Values, *string) {
	t.Helper()
	var form url.Values
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form = r.Form
		auth = r.Header.Get("Authorization")
		if status != 0 {
			w.WriteHeader(status)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &form, &auth
}

func TestRevoker(t *testing.T) {
	t.Parallel()
	srv, form, auth := revocationServer(t, 0)

	r := oauth2client.NewRevoker(srv.URL, "svc", "s3cr3t")
	err := r.Revoke(t.Context(), "tok-1", oauth2client.WithTokenTypeHint(oauth2client.TokenTypeHintAccessToken))
	require.NoError(t, err)

	require.Equal(t, "tok-1", form.Get("token"))
	require.Equal(t, "access_token", form.Get("token_type_hint"))
	require.NotEmpty(t, *auth) // default auth style presents client credentials via Basic
}

func TestRevokerErrors(t *testing.T) {
	t.Parallel()
	r := oauth2client.NewRevoker("https://idp.invalid/revoke", "svc", "s3cr3t")
	require.ErrorIs(t, r.Revoke(t.Context(), ""), oauth2client.ErrRevocation)

	srv, _, _ := revocationServer(t, http.StatusBadRequest)
	bad := oauth2client.NewRevoker(srv.URL, "svc", "s3cr3t")
	require.ErrorIs(t, bad.Revoke(t.Context(), "tok"), oauth2client.ErrRevocation)
}
