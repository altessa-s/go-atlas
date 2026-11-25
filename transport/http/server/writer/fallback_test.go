// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer_test

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/codec"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

type failingResponseWriter struct{ *httptest.ResponseRecorder }

func (failingResponseWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("Bearer standalone-writer-token https://writer.test/?api_key=SECRET: %w", io.ErrClosedPipe)
}

func TestFallbackWriterErrorRedaction(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	wr := writer.New(writer.WithRegistry(codec.NewRegistry()), writer.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))))
	out := failingResponseWriter{httptest.NewRecorder()}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	err := wr.WriteError(out, req, io.EOF)
	require.ErrorIs(t, err, writer.ErrNoCodecAvailable)
	require.Equal(t, http.StatusInternalServerError, out.Code)
	require.Contains(t, logs.String(), "fallback response write failed")
	require.NotContains(t, logs.String(), "standalone-writer-token")
	require.NotContains(t, logs.String(), "SECRET")
	require.Contains(t, logs.String(), "closed_pipe")
}
