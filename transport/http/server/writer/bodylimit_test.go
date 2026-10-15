// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// jsonBody returns a valid JSON document of exactly size bytes: a string
// value padded to fill the remainder.
func jsonBody(tb testing.TB, size int) []byte {
	tb.Helper()
	const overhead = len(`{"a":""}`)
	require.GreaterOrEqual(tb, size, overhead)
	return []byte(`{"a":"` + strings.Repeat("x", size-overhead) + `"}`)
}

// chunkedRequest builds a POST whose length is unknown to the server
// (ContentLength -1), the shape of a Transfer-Encoding: chunked upload.
func chunkedRequest(body io.Reader) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	return req
}

// countingReader records how many bytes were pulled from the body.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestWriter_Read_DefaultBodyLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "at limit", size: int(writer.DefaultMaxBodySize)},
		{name: "one byte over", size: int(writer.DefaultMaxBodySize) + 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out map[string]string
			err := writer.New().Read(chunkedRequest(bytes.NewReader(jsonBody(t, tc.size))), &out)
			if tc.wantErr {
				require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWriter_Read_ZeroMaxBodySizeUsesDefault(t *testing.T) {
	t.Parallel()

	var out map[string]string
	body := jsonBody(t, int(writer.DefaultMaxBodySize)+1)
	err := writer.New(writer.WithMaxBodySize(0)).Read(chunkedRequest(bytes.NewReader(body)), &out)
	require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
}

func TestWriter_Read_UnlimitedBodySize(t *testing.T) {
	t.Parallel()

	var out map[string]string
	body := jsonBody(t, int(writer.DefaultMaxBodySize)*2)
	err := writer.New(writer.WithMaxBodySize(writer.UnlimitedBodySize)).Read(chunkedRequest(bytes.NewReader(body)), &out)
	require.NoError(t, err)
	require.Len(t, out["a"], len(body)-len(`{"a":""}`))
}

func TestWriter_Read_MaxInt64BodySize(t *testing.T) {
	t.Parallel()

	var out map[string]string
	err := writer.New(writer.WithMaxBodySize(math.MaxInt64)).Read(chunkedRequest(bytes.NewReader(jsonBody(t, 64))), &out)
	require.NoError(t, err)
}

func TestWriter_Read_BodyLimitBoundary(t *testing.T) {
	t.Parallel()

	const limit = 32
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "below", size: limit - 1},
		{name: "exact", size: limit},
		{name: "above", size: limit + 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out map[string]string
			err := writer.New(writer.WithMaxBodySize(limit)).Read(chunkedRequest(bytes.NewReader(jsonBody(t, tc.size))), &out)
			if tc.wantErr {
				require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A declared Content-Length over the limit is rejected before the body is
// read, so an oversized upload costs neither the read nor the buffer.
func TestWriter_Read_DeclaredLengthRejectedBeforeRead(t *testing.T) {
	t.Parallel()

	body := &countingReader{r: bytes.NewReader(jsonBody(t, 64))}
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.ContentLength = 64
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := writer.New(writer.WithMaxBodySize(32)).Read(req, &out)
	require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
	require.Zero(t, body.n)
}

// A Content-Length that understates the body does not lift the limit.
func TestWriter_Read_UnderstatedLengthStillLimited(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonBody(t, 64)))
	req.ContentLength = 8
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := writer.New(writer.WithMaxBodySize(32)).Read(req, &out)
	require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
}

// The bodylimit middleware wraps the body in http.MaxBytesReader; when its
// limit trips first the writer must still report the size sentinel.
func TestWriter_Read_MaxBytesReaderError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := chunkedRequest(bytes.NewReader(jsonBody(t, 64)))
	req.Body = http.MaxBytesReader(rec, req.Body, 16)

	var out map[string]string
	err := writer.New(writer.WithMaxBodySize(1024)).Read(req, &out)
	require.ErrorIs(t, err, writer.ErrBodySizeLimitExceeded)
	require.False(t, errors.Is(err, writer.ErrReadBody))
}

func TestWriter_WriteError_BodyLimitStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status []int
		want   int
	}{
		{name: "default status", want: http.StatusRequestEntityTooLarge},
		{name: "explicit status wins", status: []int{http.StatusBadRequest}, want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wr := writer.New(writer.WithMaxBodySize(8))
			var out map[string]string
			readErr := wr.Read(chunkedRequest(bytes.NewReader(jsonBody(t, 64))), &out)
			require.ErrorIs(t, readErr, writer.ErrBodySizeLimitExceeded)

			rec := httptest.NewRecorder()
			require.NoError(t, wr.WriteError(rec, httptest.NewRequest(http.MethodPost, "/", nil), readErr, tc.status...))
			require.Equal(t, tc.want, rec.Code)
		})
	}
}
