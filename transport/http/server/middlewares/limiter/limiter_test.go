// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
