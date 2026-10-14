// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
)

type mockLimiter struct {
	info *sharedlimiter.LimitInfo
	err  error
}

func (m *mockLimiter) Limit(_ context.Context) (*sharedlimiter.LimitInfo, error) {
	return m.info, m.err
}

func TestMiddleware_Allowed(t *testing.T) {
	lim := &mockLimiter{info: &sharedlimiter.LimitInfo{Limit: 100, Remaining: 99, Reset: 1000}}
	mw := Middleware(lim)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.True(t, called, "handler should be called")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "100", rec.Header().Get("X-RateLimit-Limit"))
}

func TestMiddleware_RateLimited(t *testing.T) {
	lim := &mockLimiter{
		info: &sharedlimiter.LimitInfo{Limit: 100, Remaining: 0, Reset: 60},
		err:  sharedlimiter.ErrLimitExceeded,
	}
	mw := Middleware(lim)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.False(t, called, "handler should not be called")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestMiddleware_FallbackAllow(t *testing.T) {
	lim := &mockLimiter{err: context.DeadlineExceeded}
	mw := Middleware(lim, WithFallbackBehavior(fallback.Allow))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.True(t, called, "handler should be called on fallback allow")
}

func TestMiddleware_FallbackDeny(t *testing.T) {
	lim := &mockLimiter{err: context.DeadlineExceeded}
	mw := Middleware(lim, WithFallbackBehavior(fallback.Deny))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.False(t, called, "handler should not be called on fallback deny")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	deps := m.Dependencies()
	require.Len(t, deps, 0)
	reqDeps := m.RequiredDependencies()
	require.Len(t, reqDeps, 1)
	require.Equal(t, "realip", reqDeps[0])
}

// TestMiddleware_RetryAfterIsDelaySeconds pins that Retry-After carries the
// seconds until the window frees a slot, while X-RateLimit-Reset keeps the
// absolute Unix time.
func TestMiddleware_RetryAfterIsDelaySeconds(t *testing.T) {
	t.Parallel()
	reset := time.Now().Add(30 * time.Second).Unix()
	lim := &mockLimiter{
		info: &sharedlimiter.LimitInfo{Limit: 100, Remaining: 0, Reset: reset},
		err:  sharedlimiter.ErrLimitExceeded,
	}
	handler := Middleware(lim)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, strconv.FormatInt(reset, 10), rec.Header().Get("X-RateLimit-Reset"))
	retryAfter, err := strconv.ParseInt(rec.Header().Get("Retry-After"), 10, 64)
	require.NoError(t, err)
	require.InDelta(t, 31, retryAfter, 1)
}

func TestRetryAfterSeconds(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000, 0)
	// The reset is rounded down: an expiry at 1042.9 is reported as 1042,
	// so even from 1000.1 a client must wait 43s, not 42.
	fractional := time.Unix(1_000, 100*int64(time.Millisecond))
	tests := []struct {
		name  string
		reset int64
		want  int64
	}{
		{name: "future", reset: 1_042, want: 43},
		{name: "now", reset: 1_000, want: 1},
		{name: "past", reset: 900, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, retryAfterSeconds(tc.reset, now))
		})
	}
	require.Equal(t, int64(43), retryAfterSeconds(1_042, fractional))
}
