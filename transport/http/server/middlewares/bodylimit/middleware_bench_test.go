// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func BenchmarkMiddleware(b *testing.B) {
	mw := Middleware(1024 * 1024)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	body := strings.NewReader("test body")
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/bench", body)
		req.ContentLength = 9
		handler.ServeHTTP(rec, req)
		body.Reset("test body")
	}
}
