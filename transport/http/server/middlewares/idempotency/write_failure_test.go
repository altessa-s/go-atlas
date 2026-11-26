// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	internalidem "github.com/altessa-s/go-atlas/internal/transport/idempotency"
)

type partialResponseWriter struct{ http.ResponseWriter }

func (w partialResponseWriter) Write([]byte) (int, error) { return 2, io.ErrClosedPipe }

func TestCompletionDoesNotReplayPartialWrite(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/orders", nil)
	w := middlewares.NewResponseWriter(partialResponseWriter{httptest.NewRecorder()}, true)
	defer w.Release()
	n, err := w.Write([]byte("hello"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.Equal(t, 2, n)
	require.EqualValues(t, 2, w.BytesWritten())
	require.Equal(t, "he", w.BodyString())
	require.ErrorIs(t, w.WriteError(), io.ErrClosedPipe)
	m := New(newKeeper(), WithResponseReplay())
	record := m.completionData(req, w, false).(internalidem.Record)
	require.False(t, record.HasHTTPResponse(replayBinding(req)))
	require.Empty(t, record.Body)
}
