// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

type scopePrincipal struct {
	scopes []string
}

func newScopeEnforcer() *scope.Enforcer[*scopePrincipal] {
	reg := scope.NewRegistry()
	reg.Register("GET /files", "files:read")
	reg.Freeze()
	return scope.NewEnforcer(reg, scope.ScopeAuthorizer(
		func(p *scopePrincipal) []string { return p.scopes },
		scope.Exact(),
	))
}

func scopeKey(r *http.Request) string { return r.Method + " " + r.URL.Path }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

// serveScope runs path through ScopeMiddleware, optionally seeding the auth
// context with data (nil = unauthenticated). White-box so it can install the
// private auth context key the way the authentication middleware does.
func serveScope(h http.Handler, method, path string, data any) int {
	r := httptest.NewRequest(method, path, nil)
	if data != nil {
		r = r.WithContext(context.WithValue(r.Context(), authContextKey, data))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestScopeMiddlewareAllows(t *testing.T) {
	t.Parallel()
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	code := serveScope(h, http.MethodGet, "/files", &scopePrincipal{scopes: []string{"files:read"}})
	require.Equal(t, http.StatusOK, code)
}

func TestScopeMiddlewareDeniesMissingScope(t *testing.T) {
	t.Parallel()
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	code := serveScope(h, http.MethodGet, "/files", &scopePrincipal{scopes: []string{"files:write"}})
	require.Equal(t, http.StatusForbidden, code)
}

func TestScopeMiddlewareDeniesNoPrincipal(t *testing.T) {
	t.Parallel()
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	code := serveScope(h, http.MethodGet, "/files", nil)
	require.Equal(t, http.StatusForbidden, code)
}

func TestScopeMiddlewareDeniesWrongPrincipalType(t *testing.T) {
	t.Parallel()
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	code := serveScope(h, http.MethodGet, "/files", "not-a-principal")
	require.Equal(t, http.StatusForbidden, code)
}

func TestScopeMiddlewareDeniesUnregisteredRoute(t *testing.T) {
	t.Parallel()
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	code := serveScope(h, http.MethodGet, "/secret", &scopePrincipal{scopes: []string{"files:read"}})
	require.Equal(t, http.StatusForbidden, code)
}
