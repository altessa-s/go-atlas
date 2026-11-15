// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	obshealth "github.com/altessa-s/go-atlas/observability/health"
	grpchealth "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
)

var _ grpchealth.HealthWatcher = (*obshealth.Coordinator)(nil)

func startHealthServer(t *testing.T, coord *obshealth.Coordinator, stop <-chan struct{}) grpc_health_v1.HealthClient {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	grpchealth.New(coord).Register(gs, stop)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return grpc_health_v1.NewHealthClient(conn)
}

func newCoord(t *testing.T, opts ...obshealth.Option) *obshealth.Coordinator {
	t.Helper()
	coord := obshealth.New(opts...)
	t.Cleanup(coord.Close)
	return coord
}

func staticChecker(st obshealth.ServingStatus) obshealth.Checker {
	return obshealth.Func(func(context.Context) obshealth.ServingStatus { return st })
}

func TestHandler_Check(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	coord.RegisterService("mq", staticChecker(obshealth.StatusNotServing))
	client := startHealthServer(t, coord, nil)

	tests := []struct {
		name     string
		service  string
		want     grpc_health_v1.HealthCheckResponse_ServingStatus
		wantCode codes.Code
	}{
		{name: "overall not serving", service: "", want: grpc_health_v1.HealthCheckResponse_NOT_SERVING},
		{name: "serving service", service: "db", want: grpc_health_v1.HealthCheckResponse_SERVING},
		{name: "not serving service", service: "mq", want: grpc_health_v1.HealthCheckResponse_NOT_SERVING},
		{name: "unknown service", service: "nope", wantCode: codes.NotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, err := client.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: tc.service})
			if tc.wantCode != codes.OK {
				require.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, resp.GetStatus())
		})
	}
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	coord.RegisterService("mq", staticChecker(obshealth.StatusNotServing))
	client := startHealthServer(t, coord, nil)

	resp, err := client.List(t.Context(), &grpc_health_v1.HealthListRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetStatuses(), 2)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.GetStatuses()["db"].GetStatus())
	require.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, resp.GetStatuses()["mq"].GetStatus())
}

func TestHandler_Watch_UpdatesAndDedup(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	client := startHealthServer(t, coord, nil)

	stream, err := client.Watch(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: "db"})
	require.NoError(t, err)

	first, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, first.GetStatus())

	// The subscription is registered before the initial Send, so pushes after
	// the first Recv reach the watcher. A duplicate SERVING is suppressed.
	coord.NotifyStatusChange("db", obshealth.StatusServing)
	coord.NotifyStatusChange("db", obshealth.StatusNotServing)

	next, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, next.GetStatus())
}

func TestHandler_Watch_StopCancels(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	stop := make(chan struct{})
	client := startHealthServer(t, coord, stop)

	stream, err := client.Watch(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: "db"})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.NoError(t, err)

	close(stop)

	_, err = stream.Recv()
	require.Equal(t, codes.Canceled, status.Code(err))
}

func TestHandler_Watch_ClientCancel(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	client := startHealthServer(t, coord, nil)

	ctx, cancel := context.WithCancel(t.Context())
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{Service: "missing"})
	require.NoError(t, err)

	first, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN, first.GetStatus())

	cancel()
	_, err = stream.Recv()
	require.Equal(t, codes.Canceled, status.Code(err))
}

func TestHandler_Watch_WatcherLimit(t *testing.T) {
	t.Parallel()

	coord := newCoord(t, obshealth.WithMaxWatchersPerService(1))
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	client := startHealthServer(t, coord, nil)

	first, err := client.Watch(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: "db"})
	require.NoError(t, err)
	_, err = first.Recv()
	require.NoError(t, err)

	second, err := client.Watch(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: "db"})
	require.NoError(t, err)
	_, err = second.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestHandler_Watch_CoordinatorShutdown(t *testing.T) {
	t.Parallel()

	coord := newCoord(t)
	coord.RegisterService("db", staticChecker(obshealth.StatusServing))
	client := startHealthServer(t, coord, nil)
	coord.Close()

	stream, err := client.Watch(t.Context(), &grpc_health_v1.HealthCheckRequest{Service: "db"})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.Equal(t, codes.Unavailable, status.Code(err), "a closed coordinator is unavailable, not overloaded: %v", err)
}
