// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestNew(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	m := New(WithMeter(tc))
	require.Equal(t, "metrics", m.Name())
}
func TestMiddleware_Handler(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	m := New(WithMeter(tc))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
func TestMiddleware_Dependencies(t *testing.T) {
	t.Parallel()
	m := &Middleware{}
	require.Nil(t, m.Dependencies())
}

// TestMiddleware_TwoSubsystemsShareCollector locks the contract that two
// middlewares with distinct subsystems can register against the same
// Prometheus registry without panicking on duplicate metric registration.
func TestMiddleware_TwoSubsystemsShareCollector(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()

	require.NotPanics(t, func() {
		egrul := New(WithMeter(tc), WithMetricsSubsystem("egrul"))
		kfocus := New(WithMeter(tc), WithMetricsSubsystem("kfocus"))
		runOne := func(m *Middleware) {
			handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/x", nil)
			handler.ServeHTTP(rec, req)
		}
		runOne(egrul)
		runOne(kfocus)
	})
	require.Equal(t, float64(1),
		metricstest.GetCounterValue(t, tc, "test_egrul_server_requests_total",
			"method", "GET", "status", "200", "route", "/_unknown"))
	require.Equal(t, float64(1),
		metricstest.GetCounterValue(t, tc, "test_kfocus_server_requests_total",
			"method", "GET", "status", "200", "route", "/_unknown"))
}

// TestMiddleware_SameSubsystemReusesMetrics confirms the adapter dedupes by
// name: calling New twice with the same collector and same subsystem reuses
// the existing metric vectors without panic.
func TestMiddleware_SameSubsystemReusesMetrics(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()

	require.NotPanics(t, func() {
		_ = New(WithMeter(tc))
		_ = New(WithMeter(tc))
	})
}

func TestMiddleware_UnknownMethodsShareOneSeries(t *testing.T) {
	t.Parallel()
	m := New(WithMeter(metricstest.NewTestCollector()))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, method := range []string{"X0001", "X0002", "PROPFIND", http.MethodGet} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/test", nil))
	}

	var methods []string
	m.handles.Range(func(k, _ any) bool {
		methods = append(methods, k.(requestLabels).method)
		return true
	})
	require.ElementsMatch(t, []string{"OTHER", http.MethodGet}, methods)
}

func TestMiddlewareInformationalResponse(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	m := New(WithMeter(tc))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusCreated)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_http_server_requests_total", "method", "GET", "status", "201", "route", "/_unknown"))
}

// TestNew_WithoutCollector pins the safe no-op default: omitting
// WithMeter must neither panic nor break request handling.
func TestNew_WithoutCollector(t *testing.T) {
	t.Parallel()

	var m *Middleware
	require.NotPanics(t, func() { m = New() })

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	require.Equal(t, http.StatusTeapot, rec.Code)
}
