// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseWriter_StatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.WriteHeader(http.StatusNotFound)
	require.Equal(t, http.StatusNotFound, rw.StatusCode())
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	require.Equal(t, http.StatusOK, rw.StatusCode())
}

func TestResponseWriter_CaptureBody(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, true)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	require.Equal(t, "hello", rw.BodyString())
}

func TestResponseWriter_NoCaptureBody(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.Write([]byte("hello")) //nolint:errcheck
	require.Equal(t, 0, len(rw.Body()))
}

func TestResponseWriter_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	rw.WriteHeader(http.StatusOK)
	rw.WriteHeader(http.StatusNotFound)
	require.Equal(t, http.StatusOK, rw.StatusCode())
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec, false)
	defer rw.Release()

	require.Equal(t, rec, rw.Unwrap())
}
