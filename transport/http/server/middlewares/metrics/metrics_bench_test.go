// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkMiddleware_Handler(b *testing.B) {
	tc := testhelpers.NewTestCollector()
	m := New(WithCollector(tc))
	handler := m.Handler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
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
