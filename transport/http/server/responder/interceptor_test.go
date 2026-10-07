// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorInterceptor_PassThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)
	defer ei.Finish()

	ei.WriteHeader(http.StatusOK)
	ei.Write([]byte("ok")) //nolint:errcheck
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestErrorInterceptor_InterceptsError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)

	ei.WriteHeader(http.StatusBadRequest)
	ei.Write([]byte("bad input")) //nolint:errcheck
	ei.Finish()

	require.True(t, w.called, "error writer should be called for 4xx")
}

func TestErrorInterceptor_DoubleWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)
	defer ei.Finish()

	ei.WriteHeader(http.StatusOK)
	ei.WriteHeader(http.StatusNotFound) // should be ignored
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestErrorInterceptor_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	ei := NewErrorInterceptor(rec, req, &mockErrorWriter{})
	defer ei.Finish()

	require.Equal(t, rec, ei.Unwrap())
}

func TestErrorInterceptor_FlushStreamsWithoutRelease(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)

	_, err := ei.Write([]byte("a"))
	require.NoError(t, err)
	require.NoError(t, http.NewResponseController(ei).Flush())
	require.True(t, rec.Flushed, "Flush must reach the underlying writer")

	_, err = ei.Write([]byte("b"))
	require.NoError(t, err, "interceptor must stay usable after Flush")
	ei.Finish()

	require.Equal(t, "ab", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, w.called)
}

func TestErrorInterceptor_FlushWhileInterceptedKeepsError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/fail", nil)
	w := &mockErrorWriter{}
	ei := NewErrorInterceptor(rec, req, w)

	ei.WriteHeader(http.StatusInternalServerError)
	_, err := ei.Write([]byte("boom"))
	require.NoError(t, err)
	ei.Flush()
	require.False(t, rec.Flushed, "a buffered error response must not be committed by Flush")
	require.False(t, w.called)

	ei.Finish()
	require.True(t, w.called, "Finish must still write the structured error")
}

func TestErrorInterceptor_Hijack_NotSupported(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	ei := NewErrorInterceptor(rec, req, &mockErrorWriter{})
	defer ei.Finish()

	_, _, err := ei.Hijack()
	require.Error(t, err)
}
