// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"
)

// denyAdminRegistry allows everything except /admin, which is closed to the
// country the test resolver reports.
func denyAdminRegistry() *geoacl.Registry {
	reg := geoacl.NewRegistry(geoacl.PolicyAllow)
	reg.Register("/admin", &geoacl.AccessRule{DenyCountries: []string{"US"}})
	return reg
}

// A spelling of /admin that is not canonical used to miss the /admin rule and
// take the PolicyAllow default.
func TestMiddleware_NonCanonicalPathIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/public", http.StatusOK},
		{"/admin", http.StatusForbidden},
		{"//admin", http.StatusForbidden},
		{"/./admin", http.StatusForbidden},
		{"/public/../admin", http.StatusForbidden},
		{"/admin/.", http.StatusForbidden},
		{"/admin//", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := Middleware(newTestResolver(), denyAdminRegistry())(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

			req := requestWithIP(http.MethodGet, "/", netip.MustParseAddr("203.0.113.7"))
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantStatus == http.StatusOK, called)
		})
	}
}

// A path that is not canonical is never on the ignore list, so it cannot use
// an ignored prefix to skip the check.
func TestMiddleware_NonCanonicalPathIsNotIgnored(t *testing.T) {
	t.Parallel()

	called := false
	handler := Middleware(newTestResolver(), denyAdminRegistry(),
		WithIgnorePatterns(regexp.MustCompile(`^/public/`)),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := requestWithIP(http.MethodGet, "/", netip.MustParseAddr("203.0.113.7"))
	req.URL.Path = "/public/../admin"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, called)
}

// Behind the built-in router the handler of /admin is never reached through
// another spelling of its path: net/http.ServeMux is wrapped by the
// middleware, which refuses.
func TestMiddleware_BuiltInRoutersNeverServeNonCanonicalPath(t *testing.T) {
	t.Parallel()

	routers := map[string]func() router.Router{
		"std": func() router.Router { return std.New() },
	}
	for name, newRouter := range routers {
		for _, path := range []string{"//admin", "/./admin", "/public/../admin"} {
			t.Run(name+path, func(t *testing.T) {
				t.Parallel()
				served := false
				rt := newRouter()
				rt.Use(Middleware(newTestResolver(), denyAdminRegistry()))
				rt.HandleFunc("/admin", func(http.ResponseWriter, *http.Request) { served = true })
				rt.HandleFunc("/public", func(http.ResponseWriter, *http.Request) {})

				req := requestWithIP(http.MethodGet, "/", netip.MustParseAddr("203.0.113.7"))
				req.URL.Path = path
				rec := httptest.NewRecorder()
				rt.ServeHTTP(rec, req)

				require.False(t, served)
				require.Contains(t, []int{http.StatusForbidden, http.StatusMovedPermanently}, rec.Code)
			})
		}
	}
}
