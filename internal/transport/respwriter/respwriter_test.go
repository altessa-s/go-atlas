// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package respwriter_test

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/respwriter"
)

func TestRecorder_Status(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		codes  []int
		status int
		done   bool
	}{
		{name: "none", status: http.StatusOK},
		{name: "informational only", codes: []int{103}, status: http.StatusOK},
		{name: "informational then final", codes: []int{103, 404}, status: 404, done: true},
		{name: "first final wins", codes: []int{404, 503}, status: 404, done: true},
		{name: "switching protocols is final", codes: []int{101, 200}, status: 101, done: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := respwriter.NewRecorder(httptest.NewRecorder(), false, 0)
			defer w.Release()
			for _, c := range tc.codes {
				w.WriteHeader(c)
			}
			require.Equal(t, tc.status, w.StatusCode())
			require.Equal(t, tc.done, w.HeadersWritten())
		})
	}
}

func TestRecorder_CaptureIsBounded(t *testing.T) {
	t.Parallel()
	w := respwriter.NewRecorder(httptest.NewRecorder(), true, 8)
	defer w.Release()

	_, err := w.Write([]byte("0123456"))
	require.NoError(t, err)
	_, err = w.Write([]byte("789abc"))
	require.NoError(t, err)

	require.Equal(t, "01234567", w.BodyString())
	require.EqualValues(t, 13, w.BytesWritten())
	require.NoError(t, w.WriteError())
	require.Equal(t, http.StatusOK, w.StatusCode())
}

type failingWriter struct{ http.ResponseWriter }

func (failingWriter) Write(b []byte) (int, error) { return len(b) / 2, errors.New("broken pipe") }

func TestRecorder_PartialWriteCountsAcceptedBytes(t *testing.T) {
	t.Parallel()
	w := respwriter.NewRecorder(failingWriter{httptest.NewRecorder()}, true, 64)
	defer w.Release()

	n, err := w.Write([]byte("abcd"))
	require.Error(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, "ab", w.BodyString())
	require.EqualValues(t, 2, w.BytesWritten())
	require.Error(t, w.WriteError())
}

func TestRecorder_FlushCommitsOnlyWhenForwarded(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := respwriter.NewRecorder(rec, false, 0)
	defer w.Release()
	require.NoError(t, w.FlushError())
	require.True(t, rec.Flushed)
	require.True(t, w.HeadersWritten())
	w.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, http.StatusOK, w.StatusCode())

	// A writer that cannot flush leaves the record open.
	bare := respwriter.NewRecorder(noFlush{}, false, 0)
	defer bare.Release()
	require.ErrorIs(t, bare.FlushError(), http.ErrNotSupported)
	require.False(t, bare.HeadersWritten())
}

type noFlush struct{ http.ResponseWriter }

func (noFlush) Header() http.Header         { return http.Header{} }
func (noFlush) Write(b []byte) (int, error) { return len(b), nil }
func (noFlush) WriteHeader(int)             {}

func TestRecorder_OptionalInterfaces(t *testing.T) {
	t.Parallel()
	w := respwriter.NewRecorder(httptest.NewRecorder(), false, 0)
	defer w.Release()

	require.ErrorIs(t, w.Push("/x", nil), http.ErrNotSupported)
	_, _, err := w.Hijack()
	require.ErrorIs(t, err, http.ErrNotSupported)
	require.NotNil(t, w.Unwrap())
}

type marker struct {
	http.ResponseWriter
	marked bool
}

func (m *marker) MarkStructured() { m.marked = true }

type wrapper struct{ http.ResponseWriter }

func (w wrapper) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestMarkStructured_WalksUnwrapChain(t *testing.T) {
	t.Parallel()
	inner := &marker{ResponseWriter: httptest.NewRecorder()}
	respwriter.MarkStructured(wrapper{wrapper{inner}})
	require.True(t, inner.marked)

	respwriter.MarkStructured(httptest.NewRecorder()) // no marker: nothing to do
	respwriter.MarkStructured(nil)
}

type failFlush struct{ *httptest.ResponseRecorder }

func (failFlush) FlushError() error { return errors.New("broken pipe") }

func TestRecorder_FailedFlushMayHaveCommitted(t *testing.T) {
	t.Parallel()
	w := respwriter.NewRecorder(failFlush{httptest.NewRecorder()}, false, 0)
	defer w.Release()

	require.Error(t, w.FlushError())
	require.True(t, w.HeadersWritten(), "the status may have been sent")
	require.Error(t, w.WriteError(), "delivery failed, the response is incomplete")
	w.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, http.StatusOK, w.StatusCode())
}

type hijackable struct{ *httptest.ResponseRecorder }

func (hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func TestRecorder_HijackCommits(t *testing.T) {
	t.Parallel()
	w := respwriter.NewRecorder(hijackable{httptest.NewRecorder()}, false, 0)
	defer w.Release()

	_, _, err := w.Hijack()
	require.NoError(t, err)
	require.True(t, w.Hijacked())
	require.True(t, w.HeadersWritten())
	require.Equal(t, http.StatusSwitchingProtocols, w.StatusCode())
}
