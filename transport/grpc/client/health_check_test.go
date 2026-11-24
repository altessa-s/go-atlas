// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	client "github.com/altessa-s/go-atlas/transport/grpc/client"
	healthhandler "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestOverallHealthWatch(t *testing.T) {
	t.Parallel()
	coord := health.New()
	t.Cleanup(coord.Close)

	listener := bufconn.Listen(1 << 20)
	var calls atomic.Int32
	server, err := grpcserver.New(grpcserver.WithListener(listener), grpcserver.WithStartupVerification(0),
		grpcserver.WithGrpcOptions(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handle grpc.UnaryHandler) (any, error) {
			calls.Add(1)
			return handle(ctx, req)
		})))
	require.NoError(t, err)
	server.RegisterHandlers(healthhandler.New(coord))
	require.NoError(t, server.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
	})

	connect := func(opts ...client.Option) *grpc.ClientConn {
		dial := grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) })
		c, err := client.New(t.Context(), "passthrough:///bufnet", append(opts, client.WithInsecure(), client.WithDialOptions(dial))...)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, c.Close(context.WithoutCancel(t.Context()))) })
		conn, err := c.GetConnection(t.Context())
		require.NoError(t, err)
		return conn
	}
	conn := connect()
	check := func(conn *grpc.ClientConn) (*healthpb.HealthCheckResponse, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		return healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	}
	waitFor := func(want connectivity.State) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		for state := conn.GetState(); state != want; state = conn.GetState() {
			require.True(t, conn.WaitForStateChange(ctx, state), "state %v, want %v", state, want)
		}
	}

	resp, err := check(conn)
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
	coord.SetOverallStatus(health.StatusNotServing)
	coord.BroadcastStatus(health.StatusNotServing)
	waitFor(connectivity.TransientFailure)
	_, err = check(conn)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.EqualValues(t, 1, calls.Load())

	resp, err = check(connect(client.WithoutHealthCheck(), client.WithRetry()))
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.GetStatus())

	coord.SetOverallStatus(health.StatusServing)
	coord.BroadcastStatus(health.StatusServing)
	waitFor(connectivity.Ready)
	resp, err = check(conn)
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
	require.EqualValues(t, 3, calls.Load())
}
