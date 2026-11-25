// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc_test

import (
	"context"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

// healthHandler serves the standard health service.
type healthHandler struct{}

func (healthHandler) Register(gs grpc.ServiceRegistrar, _ <-chan struct{}) {
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

func TestRegisterHandlersAppends(t *testing.T) {
	t.Parallel()
	srv, err := grpcserver.New(grpcserver.WithListener(bufconn.Listen(1024)), grpcserver.WithStartupVerification(0))
	require.NoError(t, err)
	first, second := &countingHandler{}, &countingHandler{}
	srv.RegisterHandlers(first)
	srv.RegisterHandlers((*countingHandler)(nil), second)
	srv.RegisterHandlers()
	startServer(t, srv)
	require.EqualValues(t, 1, first.calls.Load())
	require.EqualValues(t, 1, second.calls.Load())
}

func TestWithGrpcOptionsAppliesBothCalls(t *testing.T) {
	t.Parallel()

	ln := bufconn.Listen(1 << 20)
	srv, err := grpcserver.New(
		grpcserver.WithListener(ln), grpcserver.WithStartupVerification(0),
		grpcserver.WithGrpcOptions(grpc.MaxRecvMsgSize(64)),
		grpcserver.WithGrpcOptions(grpc.MaxSendMsgSize(1)),
	)
	require.NoError(t, err)
	srv.RegisterHandlers(healthHandler{})
	startServer(t, srv)

	client := healthpb.NewHealthClient(dialBufconn(t, ln))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{Service: strings.Repeat("x", 64)})
	require.Equal(t, codes.ResourceExhausted, status.Code(err), "receive limit")
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.Equal(t, codes.ResourceExhausted, status.Code(err), "send limit")
}

type registerServiceFunc func(grpc.ServiceRegistrar)

func (f registerServiceFunc) Register(r grpc.ServiceRegistrar, _ <-chan struct{}) { f(r) }

func TestRegisterHandlersAppendsAcrossCalls(t *testing.T) {
	t.Parallel()

	ln := bufconn.Listen(1 << 20)
	srv, err := grpcserver.New(grpcserver.WithListener(ln), grpcserver.WithStartupVerification(0), grpcserver.WithReflection())
	require.NoError(t, err)
	srv.RegisterHandlers(healthHandler{})
	srv.RegisterHandlers(registerServiceFunc(func(r grpc.ServiceRegistrar) {
		r.RegisterService(&grpc.ServiceDesc{ServiceName: "test.Second"}, nil)
	}))
	startServer(t, srv)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := reflectionpb.NewServerReflectionClient(dialBufconn(t, ln))
	stream, err := client.ServerReflectionInfo(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
	}))
	resp, err := stream.Recv()
	require.NoError(t, err)
	var names []string
	for _, service := range resp.GetListServicesResponse().GetService() {
		names = append(names, service.GetName())
	}
	require.ElementsMatch(t, []string{
		"grpc.health.v1.Health", "test.Second",
		"grpc.reflection.v1.ServerReflection", "grpc.reflection.v1alpha.ServerReflection",
	}, names)
}

func TestStartUsesServiceDescriptorSnapshot(t *testing.T) {
	t.Parallel()

	ln := bufconn.Listen(1 << 20)
	srv, err := grpcserver.New(grpcserver.WithListener(ln), grpcserver.WithStartupVerification(0))
	require.NoError(t, err)
	srv.RegisterHandlers(registerServiceFunc(func(r grpc.ServiceRegistrar) {
		desc := healthpb.Health_ServiceDesc
		desc.Methods = slices.Clone(desc.Methods)
		desc.Streams = slices.Clone(desc.Streams)
		r.RegisterService(&desc, health.NewServer())
		desc.ServiceName = "test.Mutated"
		desc.Methods[0].Handler = nil
		desc.Streams[0].Handler = nil
	}))
	startServer(t, srv)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(dialBufconn(t, ln)).Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}
