// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func BenchmarkMiddleware(b *testing.B) {
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {})
	mw := Middleware(lh)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/bench", nil)
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkSlog(b *testing.B) {
	lh := Slog(slog.New(slog.DiscardHandler))
	for b.Loop() {
		lh.Log(b.Context(), "test", 200, nil)
	}
}

// BenchmarkMiddleware_SlogDisabledLevel runs successful requests through the
// Slog handler with the logger at Warn, so the Info completion record is
// dropped; the middleware should not build it.
func BenchmarkMiddleware_SlogDisabledLevel(b *testing.B) {
	l := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
	handler := Middleware(Slog(l))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/bench", nil)

	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}
