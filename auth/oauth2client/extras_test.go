// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"

	"golang.org/x/oauth2"
)

func TestPrincipal(t *testing.T) {
	t.Parallel()
	tok := (&oauth2.Token{AccessToken: "x"}).WithExtra(map[string]any{"scope": "orders:read orders:write"})
	p := oauth2client.Principal("svc", tok)
	require.Equal(t, "svc", p.Subject)
	require.Equal(t, []string{"orders:read", "orders:write"}, p.Scopes)

	// Nil token yields subject only.
	p = oauth2client.Principal("svc", nil)
	require.Equal(t, "svc", p.Subject)
	require.Empty(t, p.Scopes)
}

// staticEndpoint is a test [oauth2client.TokenEndpointSource].
type staticEndpoint string

func (s staticEndpoint) TokenEndpoint() string { return string(s) }

func TestFromDiscovery(t *testing.T) {
	t.Parallel()
	src, err := oauth2client.ClientCredentialsFromDiscovery(
		t.Context(), staticEndpoint("https://idp.example/token"), "svc", "secret")
	require.NoError(t, err)
	require.NotNil(t, src)

	ex, err := oauth2client.NewExchangerFromDiscovery(staticEndpoint("https://idp.example/token"), "svc", "secret")
	require.NoError(t, err)
	require.NotNil(t, ex)
}

func TestFromDiscoveryNoEndpoint(t *testing.T) {
	t.Parallel()
	_, err := oauth2client.ClientCredentialsFromDiscovery(t.Context(), staticEndpoint(""), "svc", "secret")
	require.ErrorIs(t, err, oauth2client.ErrNoTokenEndpoint)

	_, err = oauth2client.NewExchangerFromDiscovery(staticEndpoint(""), "svc", "secret")
	require.ErrorIs(t, err, oauth2client.ErrNoTokenEndpoint)
}

func TestExchangerRetriesOnServerError(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"access_token": "ok", "token_type": "Bearer"}))
	}))
	t.Cleanup(srv.Close)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "secret",
		oauth2client.WithRetryAttempts(5),
		oauth2client.WithRetryBaseDelay(time.Millisecond),
		oauth2client.WithRetryMaxDelay(5*time.Millisecond),
	)
	tok, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	require.NoError(t, err)
	require.Equal(t, "ok", tok.AccessToken)
	require.Equal(t, int32(3), hits.Load()) // 2 failures + 1 success
}

func TestExchangerNoRetryOnClientError(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "secret",
		oauth2client.WithRetryAttempts(5),
		oauth2client.WithRetryBaseDelay(time.Millisecond),
	)
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
	require.Equal(t, int32(1), hits.Load()) // 4xx is not retried
}
