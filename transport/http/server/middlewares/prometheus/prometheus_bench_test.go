// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
)

func BenchmarkMiddleware_Handler(b *testing.B) {
	reg := prom.NewRegistry()
	m := New(WithRegisterer(reg))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkRecorder(b *testing.B) {
	rec := httptest.NewRecorder()
	for b.Loop() {
		r := newRecorder(rec)
		r.WriteHeader(http.StatusOK)
		r.Write([]byte("ok")) //nolint:errcheck
	}
}
