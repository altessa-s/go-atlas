// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"

	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
)

// spyLimiter records the client identity the middleware handed over.
type spyLimiter struct {
	info  *sharedlimiter.LimitInfo
	err   error
	calls int
	ip    string
	token string
}

func (s *spyLimiter) Limit(ctx context.Context) (*sharedlimiter.LimitInfo, error) {
	s.calls++
	s.ip = limiters.ClientIPFromContext(ctx)
	s.token = limiters.AuthTokenFromContext(ctx)
	return s.info, s.err
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "limiter", limiter.Name())
}

func TestHandler_PassesClientIdentityToLimiter(t *testing.T) {
	t.Parallel()

	spy := &spyLimiter{info: &sharedlimiter.LimitInfo{Limit: 10, Remaining: 9, Reset: 1700000000}}
	var logs bytes.Buffer
	h := limiter.Middleware(spy, limiter.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("Authorization", "bearer abc.def")
	req = req.WithContext(clientip.NewContext(req.Context(), netip.MustParseAddr("192.0.2.5")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "192.0.2.5", spy.ip)
	require.Equal(t, "abc.def", spy.token, "Bearer scheme matched case-insensitively")
	require.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
	require.Equal(t, "1700000000", rec.Header().Get("X-RateLimit-Reset"))
	require.Contains(t, logs.String(), "rate limit check passed")
}

func TestHandler_BearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		headers []string
		want    string
	}{
		{"valid", []string{"Bearer abc.def=="}, "abc.def=="},
		{"lowercase", []string{"bearer xyz"}, "xyz"},
		{"missing", nil, ""},
		{"empty", []string{""}, ""},
		{"wrong scheme", []string{"Basic abc"}, ""},
		{"two spaces", []string{"Bearer  abc"}, ""},
		{"invalid character", []string{"Bearer a:b"}, ""},
		{"duplicate", []string{"Bearer abc", "Bearer abc"}, ""},
		{"too long", []string{"Bearer " + strings.Repeat("a", httpx.MaxBearerLength+1)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			for _, value := range tc.headers {
				req.Header.Add("Authorization", value)
			}
			spy := &spyLimiter{}
			limiter.New(spy).Handler(okHandler()).ServeHTTP(httptest.NewRecorder(), req)
			require.Equal(t, 1, spy.calls)
			require.Equal(t, tc.want, spy.token)
		})
	}
}

func TestHandler_IgnoredPathsSkipLimiter(t *testing.T) {
	t.Parallel()

	spy := &spyLimiter{err: sharedlimiter.ErrLimitExceeded}
	h := limiter.New(spy,
		limiter.WithIgnorePaths("/healthz"),
		limiter.WithIgnorePatterns(regexp.MustCompile(`^/static/`)),
	).Handler(okHandler())

	for _, path := range []string{"/healthz", "/static/app.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
	require.Zero(t, spy.calls)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Empty(t, rec.Header().Get("Retry-After"), "no limit info means no Retry-After")
}

func TestHandler_FallbackErrorBehavior(t *testing.T) {
	t.Parallel()

	spy := &spyLimiter{err: errors.New("redis down")}
	rec := httptest.NewRecorder()
	limiter.New(spy, limiter.WithFallbackBehavior(fallback.Error)).Handler(okHandler()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
