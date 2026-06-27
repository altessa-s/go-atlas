// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkScopeMiddleware(b *testing.B) {
	h := ScopeMiddleware(newScopeEnforcer(), scopeKey)(okHandler())
	r := httptest.NewRequest(http.MethodGet, "/files", nil)
	r = r.WithContext(context.WithValue(r.Context(), authContextKey, &scopePrincipal{scopes: []string{"files:read"}}))

	for b.Loop() {
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
}
