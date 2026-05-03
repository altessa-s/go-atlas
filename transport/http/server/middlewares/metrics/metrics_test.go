// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	prom "github.com/prometheus/client_golang/prometheus"
)

func TestNew(t *testing.T) {
	coll := testhelpers.NewTestCollector(prom.NewRegistry())
	m := New(WithCollector(coll))
	require.Equal(t, "metrics", m.Name())
}

func TestMiddleware_Handler(t *testing.T) {
	coll := testhelpers.NewTestCollector(prom.NewRegistry())
	m := New(WithCollector(coll))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_Dependencies(t *testing.T) {
	m := &Middleware{}
	require.Nil(t, m.Dependencies())
}

func TestRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)

	r.WriteHeader(http.StatusNotFound)
	require.Equal(t, http.StatusNotFound, r.StatusCode())

	n, err := r.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, 5, r.Size())
}

func TestRecorder_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)
	r.WriteHeader(http.StatusOK)
	r.WriteHeader(http.StatusNotFound) // should be ignored
	require.Equal(t, http.StatusOK, r.StatusCode())
}

func TestRecorder_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	r := newRecorder(rec)
	require.Equal(t, rec, r.Unwrap())
}

// TestMiddleware_TwoSubsystemsShareCollector locks the contract that two
// middlewares with distinct subsystems can register against the same
// Prometheus registry without panicking on duplicate metric registration.
func TestMiddleware_TwoSubsystemsShareCollector(t *testing.T) {
	registry := prom.NewRegistry()
	coll := testhelpers.NewTestCollector(registry)

	require.NotPanics(t, func() {
		egrul := New(WithCollector(coll), WithMetricsSubsystem("egrul"))
		kfocus := New(WithCollector(coll), WithMetricsSubsystem("kfocus"))

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
		testhelpers.GetCounterValue(t, registry, "test_egrul_server_requests_total",
			"method", "GET", "status", "200"))
	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, registry, "test_kfocus_server_requests_total",
			"method", "GET", "status", "200"))
}

// TestMiddleware_SameSubsystemReusesMetrics confirms the adapter dedupes by
// name: calling New twice with the same collector and same subsystem reuses
// the existing metric vectors without panic.
func TestMiddleware_SameSubsystemReusesMetrics(t *testing.T) {
	registry := prom.NewRegistry()
	coll := testhelpers.NewTestCollector(registry)

	require.NotPanics(t, func() {
		_ = New(WithCollector(coll))
		_ = New(WithCollector(coll))
	})
}
