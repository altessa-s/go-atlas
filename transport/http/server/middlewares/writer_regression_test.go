// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

type headerWriter struct {
	http.ResponseWriter
	codes []int
}

func (w *headerWriter) WriteHeader(code int) { w.codes = append(w.codes, code) }

func TestResponseWriterInformationalHeaders(t *testing.T) {
	t.Parallel()
	original := &headerWriter{ResponseWriter: httptest.NewRecorder()}
	w := middlewares.NewResponseWriter(original, false)
	defer w.Release()
	w.WriteHeader(http.StatusEarlyHints)
	w.WriteHeader(http.StatusContinue)
	w.WriteHeader(http.StatusCreated)
	w.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, []int{103, 100, 201}, original.codes)
	require.Equal(t, http.StatusCreated, w.StatusCode())
}

func TestResponseWriterPreservesContentTypeDetection(t *testing.T) {
	t.Parallel()
	original := httptest.NewRecorder()
	w := middlewares.NewResponseWriter(original, false)
	defer w.Release()
	_, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=utf-8", original.Header().Get("Content-Type"))
	require.Equal(t, http.StatusOK, w.StatusCode())
	require.True(t, w.HeadersWritten())
}
