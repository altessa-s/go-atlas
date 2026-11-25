// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"bytes"
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
		recovery.WithLogRawErrors(true),
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

// TestPanicRecover_NoDefaultIgnore pins that recovery covers every path by
// default, including health, metrics, and pprof endpoints.
func TestPanicRecover_NoDefaultIgnore(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/healthz", "/metrics", "/debug/pprof/heap", "/users/metrics"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
			rr := httptest.NewRecorder()

			require.NotPanics(t, func() {
				recovery.PanicRecover(slog.New(slog.DiscardHandler))(inner).
					ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
			})
			require.Equal(t, http.StatusInternalServerError, rr.Code)
		})
	}
}

func TestPanicRecover_NilLogger(t *testing.T) {
	// Should not panic with nil logger
	handler := recovery.PanicRecover(nil)
	require.NotNil(t, handler)
}

// Recovery must keep request panics out of the process-wide panic logger.
func TestPanicRecover_SanitizesLocalAndGlobalLogs(t *testing.T) {
	var global, local bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&global, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	h := recovery.PanicRecover(slog.New(slog.NewJSONHandler(&local, nil)))(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { panic("private-panic") },
	))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/private-url", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, local.String(), "panic recovered")
	require.Contains(t, local.String(), "/_unknown")
	require.NotContains(t, local.String(), "private-panic")
	require.NotContains(t, local.String(), "private-url")
	require.NotContains(t, local.String(), "stack_trace")
	require.Empty(t, global.String())
}
