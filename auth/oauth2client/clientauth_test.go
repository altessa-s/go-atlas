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

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// assertionServer captures the token-request form and replies with a token.
func assertionServer(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form = r.Form
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "assert-token", "token_type": "Bearer", "expires_in": 3600,
		}))
	}))
	t.Cleanup(srv.Close)
	return srv, &form
}

func TestPrivateKeyJWTClientCredentials(t *testing.T) {
	t.Parallel()
	srv, form := assertionServer(t)
	priv := testhelpers.GenerateRSAKey(t, 2048)

	auth, err := oauth2client.PrivateKeyJWT("svc", jwt.SigningKey{
		KeyID:     "key-1",
		Algorithm: jwt.AlgRS256,
		Key:       priv,
	})
	require.NoError(t, err)

	src := oauth2client.ClientCredentials(t.Context(), srv.URL, "svc", "unused",
		oauth2client.WithClientAuth(auth),
		oauth2client.WithScopes("orders:read"),
	)
	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "assert-token", tok.AccessToken)

	require.Equal(t, "client_credentials", form.Get("grant_type"))
	require.Equal(t, "orders:read", form.Get("scope"))
	require.Equal(t, oauth2client.ClientAssertionTypeJWTBearer, form.Get("client_assertion_type"))
	require.Empty(t, form.Get("client_secret"))

	// The assertion verifies with the public key and carries the client identity.
	claims := parseAssertion(t, form.Get("client_assertion"), &priv.PublicKey)
	require.Equal(t, "svc", claims["iss"])
	require.Equal(t, "svc", claims["sub"])
	require.Equal(t, srv.URL, claims["aud"])
	require.NotEmpty(t, claims["jti"])
}

func TestClientSecretJWTExchange(t *testing.T) {
	t.Parallel()
	srv, form := assertionServer(t)

	auth, err := oauth2client.ClientSecretJWT("svc", "s3cr3t")
	require.NoError(t, err)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "unused", oauth2client.WithClientAuth(auth))
	_, err = ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	require.NoError(t, err)

	require.Equal(t, oauth2client.GrantTypeTokenExchange, form.Get("grant_type"))
	require.Equal(t, oauth2client.ClientAssertionTypeJWTBearer, form.Get("client_assertion_type"))
	// HS256 assertion verifies with the shared secret; no Basic secret is sent.
	claims := parseAssertion(t, form.Get("client_assertion"), []byte("s3cr3t"))
	require.Equal(t, "svc", claims["iss"])
	require.Equal(t, srv.URL, claims["aud"])
}

func TestClientAuthConstructorValidation(t *testing.T) {
	t.Parallel()
	_, err := oauth2client.PrivateKeyJWT("", jwt.SigningKey{KeyID: "k", Algorithm: jwt.AlgRS256})
	require.ErrorIs(t, err, oauth2client.ErrClientAssertion)

	_, err = oauth2client.PrivateKeyJWT("svc", jwt.SigningKey{Algorithm: jwt.AlgRS256})
	require.ErrorIs(t, err, oauth2client.ErrClientAssertion)

	_, err = oauth2client.ClientSecretJWT("svc", "")
	require.ErrorIs(t, err, oauth2client.ErrClientAssertion)
}

// parseAssertion verifies a client-assertion JWT with key and returns its claims.
func parseAssertion(t *testing.T, assertion string, key any) gojwt.MapClaims {
	t.Helper()
	claims := gojwt.MapClaims{}
	_, err := gojwt.ParseWithClaims(assertion, claims, func(*gojwt.Token) (any, error) { return key, nil })
	require.NoError(t, err)
	return claims
}
