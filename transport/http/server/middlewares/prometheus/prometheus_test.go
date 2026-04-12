// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	prom "github.com/prometheus/client_golang/prometheus"
)

func TestNew(t *testing.T) {
	reg := prom.NewRegistry()
	m := New(WithRegisterer(reg))
	require.Equal(t, "prometheus", m.Name())
}

func TestMiddleware_Handler(t *testing.T) {
	reg := prom.NewRegistry()
	m := New(WithRegisterer(reg))
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
