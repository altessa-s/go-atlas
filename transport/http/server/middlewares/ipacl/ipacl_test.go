// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"
	"github.com/altessa-s/go-atlas/transport/policy/ipacl"
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

	require.True(t, called, "handler should be called")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_Denied(t *testing.T) {
	mw := Middleware(newTestRegistry())
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, requestWithIP("GET", "/allowed", netip.MustParseAddr("192.168.1.1")))

	require.False(t, called, "handler should not be called")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestMiddleware_NoIP_FallbackDeny(t *testing.T) {
	mw := Middleware(newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/allowed", nil))

	require.False(t, called, "handler should not be called when no IP and fallback deny")
	require.Equal(t, http.StatusForbidden, rec.Code)
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

	require.True(t, called, "handler should be called on fallback allow")
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

	require.True(t, called, "handler should be called for ignored path")
}

// TestMiddleware_NoDefaultIgnore pins that the access check exempts nothing
// unless the caller configures it: infrastructure-looking paths, nested or
// not, are enforced like any other route.
func TestMiddleware_NoDefaultIgnore(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/users/metrics", "/x/pprof/y", "/admin/healthz", "/healthz", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			called := false
			handler := Middleware(newTestRegistry())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, requestWithIP("GET", path, netip.MustParseAddr("1.2.3.4")))

			require.False(t, called, "path %q must not bypass the IP access check", path)
			require.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &middleware{}
	deps := m.Dependencies()
	require.Len(t, deps, 0)
	reqDeps := m.RequiredDependencies()
	require.Len(t, reqDeps, 1)
	require.Equal(t, "realip", reqDeps[0])
}
