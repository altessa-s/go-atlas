// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeadersWritten(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		before      func(w http.ResponseWriter)
		wantWritten bool
		wantCode    int
	}{
		{"nothing_written", func(http.ResponseWriter) {}, false, http.StatusTeapot},
		{"header_written", func(w http.ResponseWriter) { w.WriteHeader(http.StatusAccepted) }, true, http.StatusAccepted},
		{"body_written", func(w http.ResponseWriter) { _, _ = w.Write([]byte("partial")) }, true, http.StatusOK},
		{"flushed", func(w http.ResponseWriter) { w.(http.Flusher).Flush() }, true, http.StatusOK},
		// The recorder keeps the 1xx code; a server sends 103, then the 418.
		{"informational_only", func(w http.ResponseWriter) { w.WriteHeader(http.StatusEarlyHints) }, false, http.StatusEarlyHints},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var written bool
			h := PanicRecover(slog.New(slog.DiscardHandler), WithHandler(
				func(_ context.Context, w http.ResponseWriter, _ *http.Request, _ any) {
					written = HeadersWritten(w)
					if !written {
						w.WriteHeader(http.StatusTeapot)
					}
				}))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tc.before(w)
				panic("boom")
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, tc.wantWritten, written)
			require.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestDefaultHandlerKeepsStartedResponse(t *testing.T) {
	t.Parallel()

	h := PanicRecover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusCreated, rec.Code, "no superfluous WriteHeader after the response started")
}

func TestHeadersWrittenUnwrapsAndIgnoresForeignWriters(t *testing.T) {
	t.Parallel()

	require.False(t, HeadersWritten(httptest.NewRecorder()))
	require.False(t, HeadersWritten(nil))
	tw := &trackingWriter{ResponseWriter: httptest.NewRecorder(), started: true}
	require.True(t, HeadersWritten(wrapper{tw}), "found through Unwrap")
}

type wrapper struct{ http.ResponseWriter }

func (w wrapper) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestHijackThroughTracker(t *testing.T) {
	t.Parallel()

	tw := &trackingWriter{ResponseWriter: httptest.NewRecorder()}
	_, _, err := tw.Hijack()
	require.ErrorIs(t, err, http.ErrNotSupported)
	require.False(t, tw.started)
}

// plainWriter supports neither flushing nor hijacking.
type plainWriter struct {
	header http.Header
	code   int
}

func (w *plainWriter) Header() http.Header         { return w.header }
func (w *plainWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *plainWriter) WriteHeader(code int)        { w.code = code }

func TestUnsupportedFlushDoesNotStartResponse(t *testing.T) {
	t.Parallel()

	tw := &trackingWriter{ResponseWriter: &plainWriter{header: http.Header{}}}
	require.ErrorIs(t, http.NewResponseController(tw).Flush(), http.ErrNotSupported, "the flush error reaches the caller")
	tw.Flush()
	require.False(t, tw.started)
}

// panickingWriter rejects every status code, as net/http does for an
// invalid one.
type panickingWriter struct{ plainWriter }

func (w *panickingWriter) WriteHeader(int) { panic("invalid WriteHeader code") }

func TestRejectedWriteHeaderDoesNotStartResponse(t *testing.T) {
	t.Parallel()

	tw := &trackingWriter{ResponseWriter: &panickingWriter{plainWriter{header: http.Header{}}}}
	require.Panics(t, func() { tw.WriteHeader(1000) })
	require.False(t, tw.started)
}

// failingFlusher commits the headers, then fails the flush, as net/http does
// on a broken connection.
type failingFlusher struct{ plainWriter }

func (w *failingFlusher) FlushError() error {
	w.code = http.StatusOK
	return errors.New("broken pipe")
}

func TestFailedFlushStartsResponse(t *testing.T) {
	t.Parallel()

	tw := &trackingWriter{ResponseWriter: &failingFlusher{plainWriter{header: http.Header{}}}}
	require.Error(t, tw.FlushError())
	require.True(t, tw.started, "the headers may already be on the wire")
}
