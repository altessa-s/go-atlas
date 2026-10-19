// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"

	client "github.com/altessa-s/go-atlas/transport/grpc/client"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// TestWithoutHealthCheck_RetryAgainstServerWithoutHealthService dials, with
// round-robin retry, a server that does not implement health.v1.HealthService
// (it serves only the standard grpc.health.v1.Health, used here as the
// application service) and asserts calls succeed.
func TestWithoutHealthCheck_RetryAgainstServerWithoutHealthService(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, grpchealth.NewServer())
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	c, err := client.New(t.Context(), listener.Addr().String(),
		client.WithInsecure(), client.WithRetry(), client.WithoutHealthCheck())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}
