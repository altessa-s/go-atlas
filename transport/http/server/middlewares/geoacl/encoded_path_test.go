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

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"
)

// encodedPathRouters are the built-in routers in every matching mode:
// gorilla/mux on the decoded path (default) and on the escaped path,
// net/http.ServeMux on the escaped path.
func encodedPathRouters() map[string]func() router.Router {
	return map[string]func() router.Router{
		"gorilla": func() router.Router { return gorilla.New() },
		"gorilla encoded": func() router.Router {
			rt := gorilla.New()
			rt.Underlying().UseEncodedPath()
			return rt
		},
		"std": func() router.Router { return std.New() },
	}
}

func dispatchWithIP(rt router.Router, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(clientip.NewContext(req.Context(), netip.MustParseAddr("203.0.113.7")))
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	return rec
}

// The ignore list read the decoded "/v1/healthz" while net/http.ServeMux
// dispatched the escaped "/v1%2Fhealthz" to the protected /{tenant} route, so
// the geographic check was skipped for it.
func TestMiddleware_EncodedSlashCannotBorrowIgnoreEntry(t *testing.T) {
	t.Parallel()

	for name, newRouter := range encodedPathRouters() {
		for _, target := range []string{"/v1%2Fhealthz", "/v1%2fhealthz"} {
			t.Run(name+target, func(t *testing.T) {
				t.Parallel()

				reg := geoacl.NewRegistry(geoacl.PolicyDeny)
				tenantServed := false
				rt := newRouter()
				rt.Use(Middleware(newTestResolver(), reg, WithIgnorePaths("/v1/healthz")))
				rt.HandleFunc("/v1/healthz", func(http.ResponseWriter, *http.Request) {})
				rt.HandleFunc("/{tenant}", func(http.ResponseWriter, *http.Request) { tenantServed = true })

				require.Equal(t, http.StatusOK, dispatchWithIP(rt, "/v1/healthz").Code, "the ignored path itself")

				rec := dispatchWithIP(rt, target)
				require.Equal(t, http.StatusForbidden, rec.Code)
				require.False(t, tenantServed, "the protected route must not be served")
			})
		}
	}
}

// Under an allow default "/users/alice%2Fadmin" decoded to three segments,
// missed the deny rule for ^/users/[^/]+$ and was still dispatched to
// /users/{id} by a router matching the escaped path.
func TestMiddleware_EncodedSlashCannotEvadeDenyRule(t *testing.T) {
	t.Parallel()

	for name, newRouter := range encodedPathRouters() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reg := geoacl.NewRegistry(geoacl.PolicyAllow)
			reg.RegisterPattern(regexp.MustCompile(`/users/[^/]+`), &geoacl.AccessRule{DenyCountries: []string{"US"}})
			served := false
			rt := newRouter()
			rt.Use(Middleware(newTestResolver(), reg))
			rt.HandleFunc("/users/{id}", func(http.ResponseWriter, *http.Request) { served = true })
			rt.HandleFunc("/users/{id}/{action}", func(http.ResponseWriter, *http.Request) {})

			for _, target := range []string{"/users/alice", "/users/alice%2Fadmin", "/users/alice%2fadmin", "/users/%2E%2E", "/users/x%2F..%2F..%2Fopen"} {
				rec := dispatchWithIP(rt, target)
				require.False(t, served, "%s must not reach /users/{id}", target)
				require.NotEqual(t, http.StatusOK, rec.Code, target)
			}
		})
	}
}

// Escapes that do not change the segments stay canonical: both routers and
// the policy see the same endpoint.
func TestMiddleware_HarmlessEscapesAreServed(t *testing.T) {
	t.Parallel()

	for name, newRouter := range encodedPathRouters() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rt := newRouter()
			rt.Use(Middleware(newTestResolver(), geoacl.NewRegistry(geoacl.PolicyAllow)))
			rt.HandleFunc("/users/{id}", func(http.ResponseWriter, *http.Request) {})

			for _, target := range []string{"/users/al%20ice", "/users/caf%C3%A9", "/users/a%5Cb", "/users/a%252Fb"} {
				require.Equal(t, http.StatusOK, dispatchWithIP(rt, target).Code, target)
			}
		})
	}
}
