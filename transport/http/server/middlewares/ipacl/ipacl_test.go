// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

func newTestRegistry() *ipacl.Registry {
	reg := ipacl.NewRegistry(ipacl.PolicyDeny)
	reg.Register("/allowed", &ipacl.AccessRule{
		Allowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})
	reg.Register("/denied", &ipacl.AccessRule{
		Denylist: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
	})
	return reg
}

func requestWithIP(method, path string, ip netip.Addr) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	ctx := clientip.NewContext(r.Context(), ip)
	return r.WithContext(ctx)
}

func TestMiddleware_Allowed(t *testing.T) {
	mw := Middleware(newTestRegistry())
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
	mw := Middleware(newTestRegistry())
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("192.168.1.1")))

	if called {
		t.Fatal("handler should not be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestMiddleware_NoIP_FallbackDeny(t *testing.T) {
	mw := Middleware(newTestRegistry(), WithFallbackBehavior(fallback.Deny))
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
	mw := Middleware(newTestRegistry(), WithFallbackBehavior(fallback.Allow))
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

func TestMiddleware_IgnoredPath(t *testing.T) {
	mw := New(newTestRegistry(), WithIgnorePaths("/denied"))
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
