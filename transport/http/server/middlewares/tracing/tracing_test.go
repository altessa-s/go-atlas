// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestNew_NilTracer(t *testing.T) {
	m := New(nil)
	require.NotNil(t, m)
	require.Equal(t, "tracing", m.Name())
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := New(nil)
	deps := m.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "realip", deps[0])
}

func TestTracingHandler(t *testing.T) {
	handler := TracingHandler(tracing.Noop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestWrapHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := WrapHandler(tracing.Noop(), inner)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestExtractClientIP_FromRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	ip := extractClientIP(req)
	require.Equal(t, "192.168.1.1", ip)
}

func TestDefaultSpanName(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/users", nil)
	name := defaultSpanName(req)
	require.Equal(t, "POST /api/users", name)
}
