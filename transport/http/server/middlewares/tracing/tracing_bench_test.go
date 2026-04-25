// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func BenchmarkTracingHandler(b *testing.B) {
	handler := TracingHandler(tracing.Noop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
