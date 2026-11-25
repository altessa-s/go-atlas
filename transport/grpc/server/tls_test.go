// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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

			opts := []grpcserver.Option{
				grpcserver.WithAddress("127.0.0.1:0"),
				grpcserver.WithTLSConfig(&tls.Config{
					MinVersion:   tls.VersionTLS12,
					Certificates: []tls.Certificate{cert},
				}),
			}
			if tc.prebound {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				t.Cleanup(func() { _ = ln.Close() })
				opts = append(opts, grpcserver.WithListener(ln))
			}
			srv, err := grpcserver.New(opts...)
			require.NoError(t, err)
			srv.RegisterHandlers(healthHandler{})
			require.NoError(t, srv.Start())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
			})

			creds := credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost",
			})
			conn, err := grpc.NewClient(srv.Listener().Addr().String(), grpc.WithTransportCredentials(creds))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			require.NoError(t, err)
			require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
		})
	}
}
