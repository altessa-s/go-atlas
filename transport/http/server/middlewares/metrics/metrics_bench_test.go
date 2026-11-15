// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func BenchmarkMiddleware_Handler(b *testing.B) {
	tc := metricstest.NewTestCollector()
	m := New(WithMeter(tc))
	handler := m.Handler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddleware_Handler_UniquePaths feeds REST paths with IDs, which
// never repeat, through the middleware.
func BenchmarkMiddleware_Handler_UniquePaths(b *testing.B) {
	tc := metricstest.NewTestCollector()
	m := New(WithMeter(tc))
	handler := m.Handler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest("GET", "/api/v1/users/0", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		req.URL.Path = "/api/v1/users/" + strconv.Itoa(i)
		handler.ServeHTTP(rec, req)
		i++
	}
}
