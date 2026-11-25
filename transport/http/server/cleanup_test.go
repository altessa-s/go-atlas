// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

type cleanupWriter struct {
	*writer.Writer
	calls int
}

func (w *cleanupWriter) NewReadWriter(resp http.ResponseWriter, req *http.Request) (writer.ReadWriter, func()) {
	rw, cleanup := w.Writer.NewReadWriter(resp, req)
	return rw, func() { w.calls++; cleanup() }
}

func TestHandleOwnsCleanup(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"return", "panic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			routes := std.New()
			w := &cleanupWriter{Writer: writer.New()}
			srv, err := server.New(server.WithWriter(w), server.WithRouter(routes))
			require.NoError(t, err)
			srv.Handle("/", func(rw writer.ReadWriter) {
				require.Zero(t, w.calls)
				require.Equal(t, "/", rw.Request().URL.Path)
				if name == "panic" {
					panic("handler panic")
				}
			})
			serve := func() {
				routes.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
			}
			if name == "panic" {
				require.PanicsWithValue(t, "handler panic", serve)
			} else {
				serve()
			}
			require.Equal(t, 1, w.calls)
		})
	}
}
