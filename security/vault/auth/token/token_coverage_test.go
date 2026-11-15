// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"

	vaultApi "github.com/hashicorp/vault/api"
)

func newClient(t *testing.T, h http.HandlerFunc) *vaultApi.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL, MaxRetries: 0})
	require.NoError(t, err)
	return client
}

func TestAuthenticate_LookupSelfFillsAuthFromData(t *testing.T) {
	t.Parallel()
	var gotToken, gotPath string
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Vault-Token")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"renewable":true,"ttl":3600}}`))
	})

	secret, err := token.New("  hvs.abc  ").Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, "hvs.abc", gotToken)
	require.Equal(t, "/v1/auth/token/lookup-self", gotPath)
	require.Equal(t, "hvs.abc", client.Token())
	require.NotNil(t, secret.Auth)
	require.Equal(t, "hvs.abc", secret.Auth.ClientToken)
	require.True(t, secret.Auth.Renewable)
	require.Equal(t, 3600, secret.Auth.LeaseDuration)
}

func TestAuthenticate_NonRenewableKeepsZeroLease(t *testing.T) {
	t.Parallel()
	client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ttl":3600}}`))
	})

	secret, err := token.New("hvs.abc").Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.False(t, secret.Auth.Renewable)
	require.Zero(t, secret.Auth.LeaseDuration)
}

func TestAuthenticate_KeepsExistingAuthBlock(t *testing.T) {
	t.Parallel()
	client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"other","lease_duration":7,"renewable":true}}`))
	})

	secret, err := token.New("hvs.abc").Authenticate(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, "other", secret.Auth.ClientToken)
	require.Equal(t, 7, secret.Auth.LeaseDuration)
}

func TestAuthenticate_ServerErrorIsAuthError(t *testing.T) {
	t.Parallel()
	client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
	})

	_, err := token.New("bad").Authenticate(t.Context(), client)
	require.Error(t, err)
	require.ErrorIs(t, err, auth.ErrPermissionDenied)
	authErr, ok := errors.AsType[*auth.AuthError](err)
	require.True(t, ok)
	require.Equal(t, "token", authErr.Method)
}

func TestAuthenticate_EmptyResponse(t *testing.T) {
	t.Parallel()
	client := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	_, err := token.New("hvs.abc").Authenticate(t.Context(), client)
	require.ErrorIs(t, err, auth.ErrEmptyResponse)
}
