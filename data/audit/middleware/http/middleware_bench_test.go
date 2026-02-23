// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"

	audithttp "github.com/altessa-s/go-atlas/data/audit/middleware/http"
)

func BenchmarkMiddleware(b *testing.B) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBufferSize(100000),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(2),
	)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer a.Shutdown(b.Context())

	handler := audithttp.Middleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkMiddleware_WithIgnorePaths(b *testing.B) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBufferSize(100000),
		audit.WithFlushInterval(50*time.Millisecond),
	)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer a.Shutdown(b.Context())

	handler := audithttp.Middleware(a,
		audithttp.WithIgnorePaths("/health", "/ready"),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
