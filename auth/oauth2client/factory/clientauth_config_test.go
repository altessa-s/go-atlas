// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/auth/oauth2client/factory"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// captureServer records the last token-request form and replies with a token.
func captureServer(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form = r.Form
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "t", "token_type": "Bearer", "expires_in": 3600,
		}))
	}))
	t.Cleanup(srv.Close)
	return srv, &form
}

func TestBuilderPrivateKeyJWTFromConfig(t *testing.T) {
	t.Parallel()
	srv, form := captureServer(t)
	priv := testhelpers.GenerateRSAKey(t, 2048)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	cfg := &config.OAuth2Client{
		TokenUrl:  srv.URL,
		ClientId:  "svc",
		AuthStyle: "auto",
		ClientAuth: &config.OAuth2ClientAuth{
			Method:     "private_key_jwt",
			PrivateKey: config.Secret(pemKey),
			KeyId:      "key-1",
			Algorithm:  "RS256",
		},
	}
	src, err := factory.New(cfg).Build(t.Context())
	require.NoError(t, err)
	_, err = src.Token()
	require.NoError(t, err)

	require.Equal(t, oauth2client.ClientAssertionTypeJWTBearer, form.Get("client_assertion_type"))
	require.Empty(t, form.Get("client_secret"))
	// The assertion verifies with the public key and carries the client identity.
	claims := gojwt.MapClaims{}
	_, err = gojwt.ParseWithClaims(form.Get("client_assertion"), claims,
		func(*gojwt.Token) (any, error) { return &priv.PublicKey, nil })
	require.NoError(t, err)
	require.Equal(t, "svc", claims["iss"])
	require.Equal(t, srv.URL, claims["aud"])
}

func TestBuilderClientSecretJWTFromConfig(t *testing.T) {
	t.Parallel()
	srv, form := captureServer(t)

	cfg := &config.OAuth2Client{
		TokenUrl:     srv.URL,
		ClientId:     "svc",
		ClientSecret: config.Secret("s3cr3t"),
		AuthStyle:    "auto",
		ClientAuth:   &config.OAuth2ClientAuth{Method: "client_secret_jwt"},
	}
	src, err := factory.New(cfg).Build(t.Context())
	require.NoError(t, err)
	_, err = src.Token()
	require.NoError(t, err)

	require.Equal(t, oauth2client.ClientAssertionTypeJWTBearer, form.Get("client_assertion_type"))
	claims := gojwt.MapClaims{}
	_, err = gojwt.ParseWithClaims(form.Get("client_assertion"), claims,
		func(*gojwt.Token) (any, error) { return []byte("s3cr3t"), nil })
	require.NoError(t, err)
	require.Equal(t, "svc", claims["iss"])
}

func TestBuilderClientAuthBadKey(t *testing.T) {
	t.Parallel()
	cfg := &config.OAuth2Client{
		TokenUrl:  "https://idp.example/token",
		ClientId:  "svc",
		AuthStyle: "auto",
		ClientAuth: &config.OAuth2ClientAuth{
			Method:     "private_key_jwt",
			PrivateKey: config.Secret("not-a-pem"),
			KeyId:      "key-1",
			Algorithm:  "RS256",
		},
	}
	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err)
}
