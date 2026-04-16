// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/service/dispatch"

	audithttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
)

func newBenchEngine(b *testing.B, store audit.Storage) *dispatch.Engine[*audit.Event] {
	b.Helper()
	eng, err := dispatch.NewEngine[*audit.Event](
		audit.StorageSink{Storage: store},
		dispatch.WithBufferSize[*audit.Event](100000),            //nolint:mnd // bench constant
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond), //nolint:mnd // bench constant
		dispatch.WithWorkers[*audit.Event](2),
	)
	if err != nil {
		b.Fatal(err)
	}
	return eng
}

func BenchmarkMiddleware(b *testing.B) {
	store := memory.New()
	eng := newBenchEngine(b, store)
	if err := eng.Start(); err != nil {
		b.Fatal(err)
	}
	defer eng.Shutdown(b.Context()) //nolint:errcheck // bench cleanup

	a, err := audit.New(eng)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer a.Shutdown(b.Context()) //nolint:errcheck // bench cleanup

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
	eng := newBenchEngine(b, store)
	if err := eng.Start(); err != nil {
		b.Fatal(err)
	}
	defer eng.Shutdown(b.Context()) //nolint:errcheck // bench cleanup

	a, err := audit.New(eng)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer a.Shutdown(b.Context()) //nolint:errcheck // bench cleanup

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
