// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// healthHandler serves the standard health service.
type healthHandler struct{}

func (healthHandler) Register(gs *grpc.Server, _ <-chan struct{}) {
	healthpb.RegisterHealthServer(gs, health.NewServer())
}

// dialBufconn returns a client connection over ln.
func dialBufconn(t *testing.T, ln *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// startServer starts srv and shuts it down when the test ends, returning how
// long Start took.
func startServer(t *testing.T, srv *grpcserver.Server) time.Duration {
	t.Helper()
	start := time.Now()
	require.NoError(t, srv.Start())
	took := time.Since(start)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
	})
	return took
}

func TestWithListenerAndNoStartupCheck(t *testing.T) {
	t.Parallel()

	ln := bufconn.Listen(1 << 20)
	srv, err := grpcserver.New(grpcserver.WithListener(ln), grpcserver.WithStartupVerification(0))
	require.NoError(t, err)
	srv.RegisterHandlers(healthHandler{})

	took := startServer(t, srv)
	require.Less(t, took, time.Second, "no startup wait")
	require.Equal(t, ln.Addr().String(), srv.Address(), "the injected listener is served")

	resp, err := healthpb.NewHealthClient(dialBufconn(t, ln)).Check(t.Context(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}

func TestWithStartupVerification(t *testing.T) {
	t.Parallel()

	const check = 150 * time.Millisecond
	ln := bufconn.Listen(1 << 20)
	srv, err := grpcserver.New(grpcserver.WithListener(ln), grpcserver.WithStartupVerification(check))
	require.NoError(t, err)

	took := startServer(t, srv)
	require.GreaterOrEqual(t, took, check)
	require.Less(t, took, grpcserver.DefaultStartupVerification, "shorter than the default check")
}
