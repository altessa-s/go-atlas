// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"

	sharedid "github.com/altessa-s/go-atlas/transport/requestid"
)

func BenchmarkRecoveryWithRequestID(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := recovery.New(logger).Handler(requestid.New(sharedid.NewGenerator()).Handler(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	))
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "550e8400-e29b-41d4-a716-446655440000")
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}
