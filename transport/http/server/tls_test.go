// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

func TestServeTLS(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		prebound bool
	}{
		{name: "automatic_listener"},
		{name: "prebound_listener", prebound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, cert := testhelpers.SelfSignedCert(t)
			ca, err := x509.ParseCertificate(cert.Certificate[1])
			require.NoError(t, err)
			roots := x509.NewCertPool()
			roots.AddCert(ca)

			opts := []server.Option{
				server.WithRouter(std.New()),
				server.WithAddress("127.0.0.1:0"),
				server.WithTLSConfig(&tls.Config{
					MinVersion:   tls.VersionTLS12,
					Certificates: []tls.Certificate{cert},
				}),
			}
			if tc.prebound {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				t.Cleanup(func() { _ = ln.Close() })
				opts = append(opts, server.WithListener(ln))
			}
			srv, err := server.New(opts...)
			require.NoError(t, err)
			srv.Handle("/ready", func(rw writer.ReadWriter) {
				rw.ResponseWriter().WriteHeader(http.StatusNoContent)
			})
			require.NoError(t, srv.Start())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
			})

			transport := &http.Transport{TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost",
			}}
			t.Cleanup(transport.CloseIdleConnections)
			cl := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+srv.Listener().Addr().String()+"/ready", nil)
			require.NoError(t, err)
			resp, err := cl.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}
