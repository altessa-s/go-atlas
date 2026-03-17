// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"
)

type mockResolver struct {
	geo geoacl.GeoInfo
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ netip.Addr) (geoacl.GeoInfo, error) {
	return m.geo, m.err
}

func newTestResolver() *mockResolver {
	return &mockResolver{geo: geoacl.GeoInfo{ContinentCode: "NA", CountryCode: "US", RegionCode: "CA"}}
}

func newTestRegistry() *geoacl.Registry {
	reg := geoacl.NewRegistry(geoacl.PolicyDeny)
	reg.Register("/allowed", &geoacl.AccessRule{
		AllowCountries: []string{"US"},
	})
	reg.Register("/denied", &geoacl.AccessRule{
		DenyCountries: []string{"US"},
	})
	return reg
}

func requestWithIP(method, path string, ip netip.Addr) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	ctx := clientip.NewContext(r.Context(), ip)
	return r.WithContext(ctx)
}

func TestMiddleware_Allowed(t *testing.T) {
	mw := Middleware(newTestResolver(), newTestRegistry())
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("10.1.1.1")))

	if !called {
		t.Fatal("handler should be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestMiddleware_Denied(t *testing.T) {
	mw := Middleware(newTestResolver(), newTestRegistry())
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/denied", netip.MustParseAddr("10.1.1.1")))

	if called {
		t.Fatal("handler should not be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestMiddleware_NoIP_FallbackDeny(t *testing.T) {
	mw := Middleware(newTestResolver(), newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/allowed", nil))

	if called {
		t.Fatal("handler should not be called when no IP and fallback deny")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestMiddleware_NoIP_FallbackAllow(t *testing.T) {
	mw := Middleware(newTestResolver(), newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/allowed", nil))

	if !called {
		t.Fatal("handler should be called on fallback allow")
	}
}

func TestMiddleware_ResolverError_FallbackDeny(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	mw := Middleware(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("10.1.1.1")))

	if called {
		t.Fatal("handler should not be called on resolver error with fallback deny")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestMiddleware_ResolverError_FallbackAllow(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	mw := Middleware(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("10.1.1.1")))

	if !called {
		t.Fatal("handler should be called on resolver error with fallback allow")
	}
}

func TestMiddleware_ResolverError_FallbackError(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	mw := Middleware(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Error))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("10.1.1.1")))

	if called {
		t.Fatal("handler should not be called on resolver error with fallback error")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestMiddleware_IgnoredPath(t *testing.T) {
	mw := New(newTestResolver(), newTestRegistry(), WithIgnorePaths("/denied"))
	called := false
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/denied", netip.MustParseAddr("1.2.3.4")))

	if !called {
		t.Fatal("handler should be called for ignored path")
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "realip" {
		t.Fatalf("Dependencies() = %v, want [realip]", deps)
	}
}
