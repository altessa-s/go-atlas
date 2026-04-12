// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
)

func TestPanicRecover_NoPanic_StatusOK(t *testing.T) {
	handler := recovery.PanicRecover(slog.Default())

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	handler(inner).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())
}

func TestPanicRecover_WithAllOptions(t *testing.T) {
	handler := recovery.PanicRecover(
		slog.Default(),
		recovery.WithLogStack(),
		recovery.WithIgnorePaths("/health", "/ready"),
	)
	require.NotNil(t, handler)
}

func TestPanicRecover_IgnorePaths_NoPanic(t *testing.T) {
	handler := recovery.PanicRecover(slog.Default(), recovery.WithIgnorePaths("/health"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Non-ignored path
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	handler(inner).ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Ignored path
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler(inner).ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
}

func TestPanicRecover_NilLogger(t *testing.T) {
	// Should not panic with nil logger
	handler := recovery.PanicRecover(nil)
	require.NotNil(t, handler)
}
