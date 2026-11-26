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
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// Streaming and connection upgrades keep working behind the response recorder.
func TestResponseWriterStreamsAndHijacks(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := middlewares.NewResponseWriter(rec, false)
	defer w.Release()

	_, ok := any(w).(http.Hijacker)
	require.True(t, ok, "the wrapper offers Hijack")

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	require.NoError(t, writer.New().WriteStreamWithFlush(w, req, map[string]string{"k": "v"}))
	require.True(t, rec.Flushed, "the flush reached the client")
	require.Equal(t, http.StatusOK, w.StatusCode())
	require.True(t, w.HeadersWritten())
}

// A flush commits the status the client received; a later WriteHeader does not change it.
func TestResponseWriterFlushCommitsStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := middlewares.NewResponseWriter(rec, false)
	defer w.Release()

	require.NoError(t, http.NewResponseController(w).Flush())
	w.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, http.StatusOK, w.StatusCode())
	require.Equal(t, http.StatusOK, rec.Code)
}
