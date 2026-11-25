// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
)

func TestHTTPServerGeneralOptionsAndErrorLogger(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	srv, err := New(WithRouter(std.New()), WithAddress("127.0.0.1:0"), WithLogger(logger))
	require.NoError(t, err)
	srv.RegisterMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Through-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	})
	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
	})

	require.True(t, srv.http.DisableGeneralOptionsHandler)
	address := srv.Listener().Addr().String()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(t.Context(), "tcp", address)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.WriteString(conn, "OPTIONS * HTTP/1.1\r\nHost: "+address+"\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodOptions})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, "true", resp.Header.Get("X-Through-Middleware"))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	require.NotNil(t, srv.http.ErrorLog)
	const peer = "192.0.2.89:32145"
	srv.http.ErrorLog.Printf("http: TLS handshake error from %s: %s", peer, "diagnostic-secret")
	require.NotContains(t, logs.String(), peer)
	require.NotContains(t, logs.String(), "diagnostic-secret")
	require.Contains(t, logs.String(), "HTTP server error")
}
