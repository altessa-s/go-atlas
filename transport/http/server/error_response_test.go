// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// typedError carries a machine code, a client message and a status.
type typedError struct{}

func (typedError) Error() string   { return "internal detail" }
func (typedError) Code() string    { return "ITEM_GONE" }
func (typedError) Message() string { return "the item is gone" }
func (typedError) HTTPStatus() int { return http.StatusNotFound }

type wireError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeWireError(t *testing.T, body []byte) wireError {
	t.Helper()
	var got wireError
	require.NoError(t, json.Unmarshal(body, &got), string(body))
	return got
}

// serveThroughInterceptor serves h behind the server's error interceptor on a
// real listener, so net/http enforces header and Content-Length rules.
func serveThroughInterceptor(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := &Server{writer: writer.New()}
	srv := httptest.NewServer(s.errorInterceptorMiddleware()(h))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, body
}

// A response the writer already structured is not re-encoded from its own JSON text.
func TestErrorInterceptor_KeepsStructuredWriterError(t *testing.T) {
	t.Parallel()
	s := &Server{writer: writer.New()}
	srv := serveThroughInterceptor(t, func(w http.ResponseWriter, r *http.Request) {
		_ = s.writer.Write(w, r, typedError{})
	})

	resp, body := get(t, srv.URL)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	got := decodeWireError(t, body)
	require.Equal(t, "ITEM_GONE", got.Error.Code)
	require.Equal(t, "the item is gone", got.Error.Message)
}

// The same holds when the handler passes an explicit status.
func TestErrorInterceptor_KeepsStructuredWriteError(t *testing.T) {
	t.Parallel()
	s := &Server{writer: writer.New()}
	srv := serveThroughInterceptor(t, func(w http.ResponseWriter, r *http.Request) {
		_ = s.writer.WriteError(w, r, typedError{}, http.StatusConflict)
	})

	resp, body := get(t, srv.URL)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	got := decodeWireError(t, body)
	require.Equal(t, "ITEM_GONE", got.Error.Code)
	require.Equal(t, "the item is gone", got.Error.Message)
}

// A handler's stale Content-Length must not survive the rewrite.
func TestErrorInterceptor_DropsStaleEntityHeaders(t *testing.T) {
	t.Parallel()
	srv := serveThroughInterceptor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.Header().Set("ETag", `"stale"`)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "oops!")
	})

	resp, body := get(t, srv.URL)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.Empty(t, resp.Header.Get("ETag"))
	got := decodeWireError(t, body)
	require.Equal(t, "Bad Gateway", got.Error.Message)
	require.Equal(t, strconv.Itoa(len(body)), resp.Header.Get("Content-Length"))
}

func TestErrorInterceptor_StatusCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		codes  []int
		status int
	}{
		{name: "informational then error", codes: []int{http.StatusEarlyHints, http.StatusNotFound}, status: http.StatusNotFound},
		{name: "first error wins", codes: []int{http.StatusNotFound, http.StatusServiceUnavailable}, status: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := serveThroughInterceptor(t, func(w http.ResponseWriter, _ *http.Request) {
				for _, c := range tc.codes {
					w.WriteHeader(c)
				}
				_, _ = io.WriteString(w, "body")
			})
			resp, _ := get(t, srv.URL)
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

// Writer.WriteError honors the error's own status and resolves the message
// from the status that is actually sent.
func TestWriterWriteError_StatusAndMessage(t *testing.T) {
	t.Parallel()
	w := writer.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	require.NoError(t, w.WriteError(rec, req, errWithStatus{status: http.StatusNotFound}))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "Not Found", decodeWireError(t, rec.Body.Bytes()).Error.Message)

	rec = httptest.NewRecorder()
	require.NoError(t, w.WriteError(rec, req, errors.New("x"), http.StatusNotFound))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "Not Found", decodeWireError(t, rec.Body.Bytes()).Error.Message)
}

type errWithStatus struct{ status int }

func (e errWithStatus) Error() string   { return "status " + strconv.Itoa(e.status) }
func (e errWithStatus) HTTPStatus() int { return e.status }
