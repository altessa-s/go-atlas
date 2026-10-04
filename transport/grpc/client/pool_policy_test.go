// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/grpc/client/pool"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	client "github.com/altessa-s/go-atlas/transport/grpc/client"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestPooledClientPreservesTLSAndInterceptors(t *testing.T) {
	t.Parallel()
	ca := testhelpers.NewCA(t)
	cert := ca.SignLeaf(t, testhelpers.WithDNSNames("localhost"))
	for _, encrypted := range []bool{false, true} {
		name := "plaintext"
		if encrypted {
			name = "tls"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			var opts []grpc.ServerOption
			if encrypted {
				opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})))
			}
			server := grpc.NewServer(opts...)
			healthpb.RegisterHealthServer(server, grpchealth.NewServer())
			t.Cleanup(server.Stop)
			go func() { _ = server.Serve(listener) }()
			p := pool.New(pool.WithSize(1))
			stop, err := p.Start(t.Context())
			require.NoError(t, err)
			t.Cleanup(stop)
			var called atomic.Bool
			c, err := client.New(t.Context(), listener.Addr().String(), client.WithPool(p),
				client.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca.Pool, ServerName: "localhost"}),
				client.WithDialOptions(grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
					called.Store(true)
					return invoker(ctx, method, req, reply, cc, opts...)
				})),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close(t.Context()) })
			conn, err := c.GetConnection(t.Context())
			require.NoError(t, err)
			defer c.ReturnConnection(conn)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			if encrypted {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.True(t, called.Load(), "pooled calls must run the client's interceptor chain")
		})
	}
}
