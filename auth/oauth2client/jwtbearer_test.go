// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"golang.org/x/oauth2"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// bearerServer is a JWT-bearer token endpoint double. reply builds the response
// of the n-th request (1-based); forms records every received form.
type bearerServer struct {
	*httptest.Server

	hits  atomic.Int32
	mu    sync.Mutex
	forms []url.Values
}

// newBearerServer starts a token endpoint answering each request with reply.
func newBearerServer(t *testing.T, reply func(w http.ResponseWriter, r *http.Request, n int32)) *bearerServer {
	t.Helper()
	s := &bearerServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := s.hits.Add(1)
		if err := r.ParseForm(); err == nil {
			s.mu.Lock()
			s.forms = append(s.forms, r.PostForm)
			s.mu.Unlock()
		}
		reply(w, r, n)
	}))
	t.Cleanup(s.Close)
	return s
}

// lastForm returns the most recently received form.
func (s *bearerServer) lastForm(t *testing.T) url.Values {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotEmpty(t, s.forms)
	return s.forms[len(s.forms)-1]
}

// writeToken replies with a JSON token response.
func writeToken(w http.ResponseWriter, fields map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fields)
}

// okToken replies with access token "tok-<n>" valid for expiresIn seconds.
func okToken(expiresIn int) func(http.ResponseWriter, *http.Request, int32) {
	return func(w http.ResponseWriter, _ *http.Request, n int32) {
		writeToken(w, map[string]any{
			"access_token": "tok-" + string(rune('0'+n)), "token_type": "Bearer", "expires_in": expiresIn,
		})
	}
}

// rsaKey returns an RS256 signing key with the given key id.
func rsaKey(t *testing.T, kid string) (jwt.SigningKey, *rsa.PrivateKey) {
	t.Helper()
	priv := testhelpers.GenerateRSAKey(t, 2048)
	return jwt.SigningKey{KeyID: kid, Algorithm: jwt.AlgRS256, Key: priv}, priv
}

func TestJWTBearerExchange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kid     string
		scopes  []string
		wantKid bool
	}{
		{name: "key id and scopes", kid: "key-1", scopes: []string{"a", "b"}, wantKid: true},
		{name: "empty key id omits kid", kid: "", scopes: []string{"cloud-platform"}},
		{name: "no scopes", kid: "key-2", wantKid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newBearerServer(t, okToken(3600))
			key, priv := rsaKey(t, tc.kid)
			clk := clock.NewFake(time.Now())

			b, err := oauth2client.NewJWTBearer(srv.URL, "svc@example.iam", key,
				oauth2client.WithJWTBearerScopes(tc.scopes...), oauth2client.WithJWTBearerClock(clk))
			require.NoError(t, err)

			tok, err := b.Token(t.Context())
			require.NoError(t, err)
			require.Equal(t, "tok-1", tok.AccessToken)
			require.Equal(t, clk.Now().Add(time.Hour), tok.Expiry)

			form := srv.lastForm(t)
			require.Equal(t, oauth2client.GrantTypeJWTBearer, form.Get("grant_type"))
			require.Empty(t, form.Get("client_id"))

			raw := form.Get("assertion")
			claims := parseAssertion(t, raw, &priv.PublicKey)
			require.Equal(t, "svc@example.iam", claims["iss"])
			require.Equal(t, srv.URL, claims["aud"])
			require.InDelta(t, float64(clk.Now().Unix()), claims["iat"], 0)
			require.InDelta(t, float64(clk.Now().Add(time.Hour).Unix()), claims["exp"], 0)
			if len(tc.scopes) > 0 {
				require.Equal(t, strings.Join(tc.scopes, " "), claims["scope"])
			} else {
				require.NotContains(t, claims, "scope")
			}

			hdr, _, err := gojwt.NewParser().ParseUnverified(raw, gojwt.MapClaims{})
			require.NoError(t, err)
			require.Equal(t, "RS256", hdr.Header["alg"])
			if tc.wantKid {
				require.Equal(t, tc.kid, hdr.Header["kid"])
			} else {
				require.NotContains(t, hdr.Header, "kid")
			}
		})
	}
}

func TestJWTBearerCachesUntilEarlyExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		expiresIn int
		early     time.Duration
		// fresh is the last instant (after issue) the cached token is served;
		// one second later a new token is fetched.
		fresh time.Duration
	}{
		{name: "default margin", expiresIn: 600, early: 0, fresh: 10*time.Minute - 11*time.Second},
		{name: "five minute margin", expiresIn: 600, early: 5 * time.Minute, fresh: 5*time.Minute - time.Second},
		{name: "margin capped at half a short lifetime", expiresIn: 60, early: 5 * time.Minute, fresh: 30*time.Second - time.Second},
		{name: "default margin capped for a ten second token", expiresIn: 10, early: 0, fresh: 5*time.Second - time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newBearerServer(t, okToken(tc.expiresIn))
			key, _ := rsaKey(t, "k")
			clk := clock.NewFake(time.Now())

			b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key,
				oauth2client.WithJWTBearerClock(clk), oauth2client.WithJWTBearerEarlyExpiry(tc.early))
			require.NoError(t, err)

			tok, err := b.Token(t.Context())
			require.NoError(t, err)
			require.Equal(t, "tok-1", tok.AccessToken)

			clk.Advance(tc.fresh)
			tok, err = b.Token(t.Context())
			require.NoError(t, err)
			require.Equal(t, "tok-1", tok.AccessToken)
			require.Equal(t, int32(1), srv.hits.Load())

			clk.Advance(time.Second)
			tok, err = b.Token(t.Context())
			require.NoError(t, err)
			require.Equal(t, "tok-2", tok.AccessToken)
			require.Equal(t, int32(2), srv.hits.Load())
		})
	}
}

func TestJWTBearerRejectsBadResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields map[string]any
		raw    string
	}{
		{name: "missing expires_in", fields: map[string]any{"access_token": "x", "token_type": "Bearer"}},
		{name: "zero expires_in", fields: map[string]any{"access_token": "x", "expires_in": 0}},
		{name: "negative expires_in", fields: map[string]any{"access_token": "x", "expires_in": -5}},
		{name: "missing access_token", fields: map[string]any{"expires_in": 3600}},
		{name: "not JSON", raw: "<html>"},
		{name: "oversized body", raw: `{"access_token":"x","expires_in":3600,"pad":"` + strings.Repeat("a", 1<<17) + `"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newBearerServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
				if tc.raw != "" {
					_, _ = w.Write([]byte(tc.raw))
					return
				}
				writeToken(w, tc.fields)
			})
			key, _ := rsaKey(t, "k")
			b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
			require.NoError(t, err)

			_, err = b.Token(t.Context())
			require.ErrorIs(t, err, oauth2client.ErrTokenExchange)

			// A failure is not cached: the next call asks again.
			_, err = b.Token(t.Context())
			require.ErrorIs(t, err, oauth2client.ErrTokenExchange)
			require.Equal(t, int32(2), srv.hits.Load())
		})
	}
}

func TestJWTBearerStatusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
		wantHits int32
	}{
		{name: "invalid grant", status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"bad sig"}`,
			wantCode: "invalid_grant", wantHits: 1},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`, wantCode: "invalid_client", wantHits: 1},
		{name: "unavailable is retried", status: http.StatusServiceUnavailable, body: "down", wantHits: 4},
		{name: "throttled is retried", status: http.StatusTooManyRequests, body: "slow", wantHits: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newBearerServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			key, _ := rsaKey(t, "k")
			// Three retries after the first attempt: four requests for a transient status.
			b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key,
				oauth2client.WithJWTBearerRetryAttempts(3),
				oauth2client.WithJWTBearerRetryBaseDelay(time.Millisecond),
				oauth2client.WithJWTBearerRetryMaxDelay(2*time.Millisecond))
			require.NoError(t, err)

			_, err = b.Token(t.Context())
			require.ErrorIs(t, err, oauth2client.ErrTokenRequest)
			re, ok := errors.AsType[*oauth2.RetrieveError](err)
			require.True(t, ok)
			require.Equal(t, tc.status, re.Response.StatusCode)
			require.Equal(t, tc.wantCode, re.ErrorCode)
			require.Equal(t, tc.wantHits, srv.hits.Load())
		})
	}
}

func TestJWTBearerRetryThenSuccess(t *testing.T) {
	t.Parallel()
	srv := newBearerServer(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		okToken(3600)(w, r, n)
	})
	key, _ := rsaKey(t, "k")
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key,
		oauth2client.WithJWTBearerRetryAttempts(2), oauth2client.WithJWTBearerRetryBaseDelay(time.Millisecond))
	require.NoError(t, err)

	tok, err := b.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, "tok-2", tok.AccessToken)

	// Every attempt mints its own assertion.
	srv.mu.Lock()
	defer srv.mu.Unlock()
	require.Len(t, srv.forms, 2)
	require.NotEmpty(t, srv.forms[1].Get("assertion"))
}

func TestJWTBearerHonorsContext(t *testing.T) {
	t.Parallel()

	t.Run("deadline bounds the exchange", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		srv := newBearerServer(t, func(_ http.ResponseWriter, r *http.Request, _ int32) {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		})
		key, _ := rsaKey(t, "k")
		b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = b.Token(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, oauth2client.ErrTokenRequest)
		require.Less(t, time.Since(start), 5*time.Second)
	})

	t.Run("canceled context sends nothing", func(t *testing.T) {
		t.Parallel()
		srv := newBearerServer(t, okToken(3600))
		key, _ := rsaKey(t, "k")
		b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = b.Token(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, int32(0), srv.hits.Load())
	})

	t.Run("cached token survives a done context", func(t *testing.T) {
		t.Parallel()
		srv := newBearerServer(t, okToken(3600))
		key, _ := rsaKey(t, "k")
		b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
		require.NoError(t, err)
		_, err = b.Token(t.Context())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		tok, err := b.Token(ctx)
		require.NoError(t, err)
		require.Equal(t, "tok-1", tok.AccessToken)
	})

	t.Run("waiter gives up on its own deadline", func(t *testing.T) {
		t.Parallel()
		entered := make(chan struct{})
		release := make(chan struct{})
		srv := newBearerServer(t, func(w http.ResponseWriter, r *http.Request, n int32) {
			close(entered)
			<-release
			okToken(3600)(w, r, n)
		})
		key, _ := rsaKey(t, "k")
		b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Go(func() {
			tok, err := b.Token(t.Context())
			if assert.NoError(t, err) {
				assert.Equal(t, "tok-1", tok.AccessToken)
			}
		})
		<-entered

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		_, err = b.Token(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		close(release)
		wg.Wait()
		require.Equal(t, int32(1), srv.hits.Load())
	})

	t.Run("waiter refreshes after the holder is canceled", func(t *testing.T) {
		t.Parallel()
		entered := make(chan struct{}, 1)
		srv := newBearerServer(t, func(w http.ResponseWriter, r *http.Request, n int32) {
			if n == 1 {
				entered <- struct{}{}
				<-r.Context().Done()
				return
			}
			okToken(3600)(w, r, n)
		})
		key, _ := rsaKey(t, "k")
		b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
		require.NoError(t, err)

		holderCtx, cancelHolder := context.WithCancel(t.Context())
		var wg sync.WaitGroup
		wg.Go(func() {
			_, err := b.Token(holderCtx)
			assert.ErrorIs(t, err, context.Canceled)
		})
		<-entered

		got := make(chan *oauth2.Token, 1)
		wg.Go(func() {
			tok, err := b.Token(t.Context())
			assert.NoError(t, err)
			got <- tok
		})
		cancelHolder()
		wg.Wait()
		tok := <-got
		require.NotNil(t, tok)
		require.Equal(t, "tok-2", tok.AccessToken)
	})
}

func TestJWTBearerConcurrentRefreshNoStampede(t *testing.T) {
	t.Parallel()
	srv := newBearerServer(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		time.Sleep(20 * time.Millisecond)
		okToken(600)(w, r, n)
	})
	key, _ := rsaKey(t, "k")
	clk := clock.NewFake(time.Now())
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key, oauth2client.WithJWTBearerClock(clk))
	require.NoError(t, err)

	burst := func(want string) {
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				tok, err := b.Token(t.Context())
				if assert.NoError(t, err) {
					assert.Equal(t, want, tok.AccessToken)
				}
			})
		}
		wg.Wait()
	}

	burst("tok-1")
	require.Equal(t, int32(1), srv.hits.Load())

	// On expiry every caller sees the stale token at once; still one refresh.
	clk.Advance(10 * time.Minute)
	burst("tok-2")
	require.Equal(t, int32(2), srv.hits.Load())
}

func TestJWTBearerTokenSource(t *testing.T) {
	t.Parallel()
	srv := newBearerServer(t, okToken(3600))
	key, _ := rsaKey(t, "k")
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key)
	require.NoError(t, err)

	src := b.TokenSource(t.Context())
	tok, err := src.Token()
	require.NoError(t, err)
	require.True(t, tok.Valid())
	tok, err = src.Token()
	require.NoError(t, err)
	require.Equal(t, "tok-1", tok.AccessToken)
	require.Equal(t, int32(1), srv.hits.Load())
}

func TestJWTBearerMetrics(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	srv := newBearerServer(t, okToken(3600))
	key, _ := rsaKey(t, "k")
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key,
		oauth2client.WithJWTBearerMetrics(oauth2client.NewMetrics(tc, "")))
	require.NoError(t, err)

	for range 3 {
		_, err = b.Token(t.Context())
		require.NoError(t, err)
	}
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, fetchesMetric, "grant", "jwt_bearer", "status", "success"))
}

func TestNewJWTBearerValidation(t *testing.T) {
	t.Parallel()
	key, _ := rsaKey(t, "k")
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tests := []struct {
		name     string
		tokenURL string
		issuer   string
		key      jwt.SigningKey
	}{
		{name: "empty token URL", tokenURL: "", issuer: "svc", key: key},
		{name: "relative token URL", tokenURL: "/token", issuer: "svc", key: key},
		{name: "non-http scheme", tokenURL: "ftp://idp/token", issuer: "svc", key: key},
		{name: "empty issuer", tokenURL: "https://idp/token", key: key},
		{name: "missing key", tokenURL: "https://idp/token", issuer: "svc", key: jwt.SigningKey{Algorithm: jwt.AlgRS256}},
		{name: "symmetric algorithm", tokenURL: "https://idp/token", issuer: "svc",
			key: jwt.SigningKey{Algorithm: "HS256", Key: []byte("secret")}},
		{name: "key does not match algorithm", tokenURL: "https://idp/token", issuer: "svc",
			key: jwt.SigningKey{Algorithm: jwt.AlgRS256, Key: ecKey}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := oauth2client.NewJWTBearer(tc.tokenURL, tc.issuer, tc.key)
			require.ErrorIs(t, err, oauth2client.ErrJWTBearerConfig)
		})
	}
}

func TestJWTBearerHugeExpiresInIsClamped(t *testing.T) {
	t.Parallel()
	srv := newBearerServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		_, _ = w.Write([]byte(`{"access_token":"x","expires_in":"9223372036854775808","id_token":"id","scope":"read","issued_token_type":"access","refresh_token":"refresh"}`))
	})
	key, _ := rsaKey(t, "k")
	clk := clock.NewFake(time.Now())
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key, oauth2client.WithJWTBearerClock(clk))
	require.NoError(t, err)

	tok, err := b.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, clk.Now().Add(time.Duration(math.MaxInt64/int64(time.Second))*time.Second), tok.Expiry)
	require.Equal(t, "id", tok.Extra("id_token"))
	require.Equal(t, "read", tok.Extra("scope"))
	require.Equal(t, "access", tok.Extra("issued_token_type"))
	require.Equal(t, "refresh", tok.RefreshToken)
}

// TestJWTBearerBoundsErrorFields pins that the RFC 6749 error fields of a
// rejection are bounded like the body: RetrieveError.Error prints them whole,
// and the error reaches the returned text and the failure log.
func TestJWTBearerBoundsErrorFields(t *testing.T) {
	t.Parallel()

	const marker = "beyond-the-bound-marker"
	long := strings.Repeat("d", 300) + marker
	srv := newBearerServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusBadRequest)
		writeToken(w, map[string]any{"error": "invalid_grant", "error_description": long, "error_uri": "https://idp.example/" + long})
	})
	key, _ := rsaKey(t, "k")
	var logs bytes.Buffer
	b, err := oauth2client.NewJWTBearer(srv.URL, "svc", key,
		oauth2client.WithJWTBearerLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	require.NoError(t, err)

	_, err = b.Token(t.Context())
	require.ErrorIs(t, err, oauth2client.ErrTokenRequest)
	re, ok := errors.AsType[*oauth2.RetrieveError](err)
	require.True(t, ok)
	require.Equal(t, "invalid_grant", re.ErrorCode)
	require.NotContains(t, err.Error(), marker)
	require.NotContains(t, logs.String(), marker)
}
