// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkHTTPDrivenMiddleware(b *testing.B) {
	md := &mockDriver{}
	dm := &mockDrivenMiddleware{driver: md}
	mw := HTTPDrivenMiddleware(dm)
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)

	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkNoopDriver(b *testing.B) {
	d := NoopDriver()
	req := httptest.NewRequest("GET", "/", nil)
	for b.Loop() {
		d.PreRequest(req.Context(), req) //nolint:errcheck
	}
}
