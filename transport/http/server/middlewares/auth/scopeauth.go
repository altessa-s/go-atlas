// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"net/http"

	"github.com/altessa-s/go-atlas/auth/scope"
)

// ScopeMiddleware enforces a [scope.Enforcer] over authenticated HTTP requests.
// The verified principal of type P is read from the request context via
// [FromContext] (installed by the authentication [Middleware]); keyFunc maps the
// request to the action key registered in the enforcer's registry — typically
// the matched route pattern, e.g.:
//
//	keyFunc := func(r *http.Request) string { return r.Method + " " + r.Pattern }
//
// A request whose context carries no principal of type P, or one the enforcer
// denies, is answered with 403 Forbidden and the chain stops. ScopeMiddleware
// must run after the authentication [Middleware] that populates the context.
func ScopeMiddleware[P any](e *scope.Enforcer[P], keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context()).(P)
			if !ok {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if err := e.Enforce(p, keyFunc(r)); err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
