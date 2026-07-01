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

// tokenExchangeServer starts a token endpoint that records the last received
// form and Authorization header, and replies with a canned token response.
func tokenExchangeServer(t *testing.T, resp map[string]any, status int) (*httptest.Server, *url.Values, *string) {
	t.Helper()
	var gotForm url.Values
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.Form
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotForm, &gotAuth
}

func TestExchangerExchange(t *testing.T) {
	t.Parallel()
	srv, form, auth := tokenExchangeServer(t, map[string]any{
		"access_token":      "exchanged-token",
		"issued_token_type": oauth2client.TokenTypeAccessToken,
		"token_type":        "Bearer",
		"expires_in":        3600,
		"scope":             "orders:read orders:write",
	}, 0)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "s3cr3t")
	tok, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{
		SubjectToken: "inbound-jwt",
		Audience:     "https://downstream.internal",
		Scopes:       []string{"orders:read", "orders:write"},
	})
	require.NoError(t, err)
	require.Equal(t, "exchanged-token", tok.AccessToken)
	require.Equal(t, "Bearer", tok.TokenType)
	require.True(t, tok.Valid())
	require.Equal(t, oauth2client.TokenTypeAccessToken, tok.Extra("issued_token_type"))
	require.Equal(t, "orders:read orders:write", tok.Extra("scope"))

	require.Equal(t, oauth2client.GrantTypeTokenExchange, form.Get("grant_type"))
	require.Equal(t, "inbound-jwt", form.Get("subject_token"))
	require.Equal(t, oauth2client.TokenTypeAccessToken, form.Get("subject_token_type"))
	require.Equal(t, "https://downstream.internal", form.Get("audience"))
	require.Equal(t, "orders:read orders:write", form.Get("scope"))
	// Default auth style presents client credentials via HTTP Basic.
	require.NotEmpty(t, *auth)
	require.Empty(t, form.Get("client_secret"))
}

func TestExchangerExchangeDelegationAndInParamsAuth(t *testing.T) {
	t.Parallel()
	srv, form, auth := tokenExchangeServer(t, map[string]any{
		"access_token": "t",
		"token_type":   "Bearer",
	}, 0)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "s3cr3t",
		oauth2client.WithAuthStyle(oauth2.AuthStyleInParams),
	)
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{
		SubjectToken: "subj",
		ActorToken:   "actor",
		Resource:     "https://api.internal/v1",
	})
	require.NoError(t, err)

	require.Equal(t, "actor", form.Get("actor_token"))
	require.Equal(t, oauth2client.TokenTypeAccessToken, form.Get("actor_token_type"))
	require.Equal(t, "https://api.internal/v1", form.Get("resource"))
	// In-params style puts credentials in the body, not the Authorization header.
	require.Equal(t, "svc", form.Get("client_id"))
	require.Equal(t, "s3cr3t", form.Get("client_secret"))
	require.Empty(t, *auth)
}

func TestExchangerExchangeSubjectRequired(t *testing.T) {
	t.Parallel()
	ex := oauth2client.NewExchanger("https://idp.invalid/token", "svc", "s3cr3t")
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{})
	require.ErrorIs(t, err, oauth2client.ErrSubjectTokenRequired)
}

func TestExchangerExchangeErrorStatus(t *testing.T) {
	t.Parallel()
	srv, _, _ := tokenExchangeServer(t, map[string]any{"error": "invalid_request"}, http.StatusBadRequest)
	ex := oauth2client.NewExchanger(srv.URL, "svc", "s3cr3t")
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "x"})
	require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
}

func TestExchangerExchangeMissingAccessToken(t *testing.T) {
	t.Parallel()
	srv, _, _ := tokenExchangeServer(t, map[string]any{"token_type": "Bearer"}, 0)
	ex := oauth2client.NewExchanger(srv.URL, "svc", "s3cr3t")
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "x"})
	require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
}

func TestExchangerTokenSource(t *testing.T) {
	t.Parallel()
	srv, form, _ := tokenExchangeServer(t, map[string]any{
		"access_token": "src-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	}, 0)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "s3cr3t")
	ts := ex.TokenSource(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	tok, err := ts.Token()
	require.NoError(t, err)
	require.Equal(t, "src-token", tok.AccessToken)
	require.Equal(t, oauth2client.GrantTypeTokenExchange, form.Get("grant_type"))
}
