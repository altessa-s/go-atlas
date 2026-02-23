// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

	if !called {
		t.Fatal("handler should be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "100" {
		t.Fatalf("X-RateLimit-Limit = %q", rec.Header().Get("X-RateLimit-Limit"))
	}
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

	if called {
		t.Fatal("handler should not be called")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d, want 429", rec.Code)
	}
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

	if !called {
		t.Fatal("handler should be called on fallback allow")
	}
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

	if called {
		t.Fatal("handler should not be called on fallback deny")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "realip" {
		t.Fatalf("Dependencies() = %v, want [realip]", deps)
	}
}

func TestConstants(t *testing.T) {
	if StatusTooManyRequests != 429 {
		t.Fatal("wrong constant")
	}
	if StatusServiceUnavailable != 503 {
		t.Fatal("wrong constant")
	}
}
