// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkMiddleware(b *testing.B) {
	handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		New()
	}
}
