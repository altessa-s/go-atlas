// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
)

// readAll is a handler that reads the body and reports a MaxBytesReader
// overflow as 413.
var readAll = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if _, err := io.ReadAll(r.Body); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func post(t *testing.T, h http.Handler, path string, size int, chunked bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", size)))
	if chunked {
		req.ContentLength = -1
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLimitFunc(t *testing.T) {
	t.Parallel()

	limits := bodylimit.WithLimitFunc(func(r *http.Request) int64 {
		switch {
		case strings.HasPrefix(r.URL.Path, "/upload"):
			return 100
		case r.URL.Path == "/unlimited":
			return -1
		default:
			return 0 // the static limit
		}
	})
	h := bodylimit.Middleware(10, limits)(readAll)

	tests := []struct {
		name    string
		path    string
		size    int
		chunked bool
		want    int
	}{
		{"static_within", "/api", 10, false, http.StatusOK},
		{"static_declared_over", "/api", 11, false, http.StatusRequestEntityTooLarge},
		{"per_route_within", "/upload/a", 100, false, http.StatusOK},
		{"per_route_declared_over", "/upload/a", 101, false, http.StatusRequestEntityTooLarge},
		{"per_route_chunked_over", "/upload/a", 101, true, http.StatusRequestEntityTooLarge},
		{"lifted", "/unlimited", 10_000, false, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, post(t, h, tc.path, tc.size, tc.chunked).Code)
		})
	}
}

func TestLimitFuncWithoutStaticLimit(t *testing.T) {
	t.Parallel()

	h := bodylimit.Middleware(0, bodylimit.WithLimitFunc(func(*http.Request) int64 { return 5 }))(readAll)
	require.Equal(t, http.StatusRequestEntityTooLarge, post(t, h, "/", 6, false).Code)
	require.Equal(t, http.StatusOK, post(t, h, "/", 5, false).Code)
}

// htmlWriter renders rejections as an HTML page.
type htmlWriter struct{}

func (htmlWriter) WriteError(w http.ResponseWriter, _ *http.Request, err error, status ...int) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status[0])
	_, werr := io.WriteString(w, "<h1>"+err.Error()+"</h1>")
	return werr
}

func TestErrorWriter(t *testing.T) {
	t.Parallel()

	h := bodylimit.Middleware(10, bodylimit.WithErrorWriter(htmlWriter{}), bodylimit.WithRequireContentLength())(readAll)

	rec := post(t, h, "/", 11, false)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "<h1>"+bodylimit.ErrBodyTooLarge.Error()+"</h1>", rec.Body.String())

	rec = post(t, h, "/", 5, true)
	require.Equal(t, http.StatusLengthRequired, rec.Code)
	require.Contains(t, rec.Body.String(), bodylimit.ErrLengthRequired.Error())
}
