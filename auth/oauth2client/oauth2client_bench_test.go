// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// BenchmarkExchangerFormValues measures building the RFC 8693 request body, the
// per-exchange hot path.
func BenchmarkExchangerFormValues(b *testing.B) {
	e, err := NewExchanger("https://idp.example/token", "svc", "s3cr3t")
	if err != nil {
		b.Fatal(err)
	}
	req := ExchangeRequest{
		SubjectToken: "inbound-access-token",
		Audience:     "https://downstream.internal",
		Resource:     "https://api.internal/v1",
		Scopes:       []string{"orders:read", "orders:write"},
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = e.formValues(req)
	}
}

// BenchmarkParseTokenResponse measures decoding a token-exchange success body.
func BenchmarkParseTokenResponse(b *testing.B) {
	body := []byte(`{"access_token":"abc","issued_token_type":"` +
		TokenTypeAccessToken + `","token_type":"Bearer","expires_in":3600,"scope":"a b c"}`)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseTokenResponse(200, body, time.Now()); err != nil {
			b.Fatal(err)
		}
	}
}

// benchJWTBearer builds a [JWTBearer] against a token endpoint double.
func benchJWTBearer(b *testing.B) *JWTBearer {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"abc","token_type":"Bearer","expires_in":3600}`))
	}))
	b.Cleanup(srv.Close)
	key := jwt.SigningKey{KeyID: "k", Algorithm: jwt.AlgRS256, Key: testhelpers.GenerateRSAKey(b, 2048)}
	bearer, err := NewJWTBearer(srv.URL, "svc@example.iam", key, WithJWTBearerScopes("cloud-platform"))
	if err != nil {
		b.Fatal(err)
	}
	return bearer
}

// BenchmarkJWTBearerTokenCached measures the cache-hit path of
// [JWTBearer.Token], the per-request hot path once a token is issued.
func BenchmarkJWTBearerTokenCached(b *testing.B) {
	bearer := benchJWTBearer(b)
	if _, err := bearer.Token(b.Context()); err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := bearer.Token(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJWTBearerAssertion measures minting and signing one RS256 grant
// assertion, the per-refresh cost.
func BenchmarkJWTBearerAssertion(b *testing.B) {
	bearer := benchJWTBearer(b)
	now := time.Now()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := bearer.assertion(now); err != nil {
			b.Fatal(err)
		}
	}
}
