// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	metricsint "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
)

const (
	pairFirstMethod  = "/cov.v1.Pair/First"
	pairSecondMethod = "/cov.v1.Pair/Second"
)

// pairService registers two echo methods, so a test can exceed a method cap
// of one.
type pairService struct{}

func (pairService) Register(gs grpc.ServiceRegistrar, _ <-chan struct{}) {
	method := func(name, full string) grpc.MethodDesc {
		return grpc.MethodDesc{
			MethodName: name,
			Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				in := &wrapperspb.StringValue{}
				if err := dec(in); err != nil {
					return nil, err
				}
				h := func(_ context.Context, req any) (any, error) { return req, nil }
				if ic == nil {
					return h(ctx, in)
				}
				return ic(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: full}, h)
			},
		}
	}
	gs.RegisterService(&grpc.ServiceDesc{
		ServiceName: "cov.v1.Pair",
		HandlerType: (*any)(nil),
		Methods:     []grpc.MethodDesc{method("First", pairFirstMethod), method("Second", pairSecondMethod)},
	}, struct{}{})
}

// Methods seen after metrics.maxMethods distinct ones share the overflow
// series instead of minting their own.
func TestWithMetricsInterceptor_AppliesMaxMethodsFromConfig(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Metrics: &grpcconfig.MetricsInterceptor{
			BaseInterceptor: on, Subsystem: "edge", StreamSamplingRate: 1,
			StreamSamplingStrategy: grpcconfig.MetricsPerMessageSampling, MaxMethods: 1,
		},
	}}
	col := metricstest.NewTestCollector()

	lis := bufconn.Listen(1 << 20)
	srv, err := factory.New(cfg).UseMeter(col).WithMetricsInterceptor().
		UseServerOptions(grpcserver.WithListener(lis), grpcserver.WithStartupVerification(0)).Build()
	require.NoError(t, err)
	srv.RegisterHandlers(pairService{})
	require.NoError(t, srv.Start())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	for _, method := range []string{pairFirstMethod, pairSecondMethod} {
		require.NoError(t, conn.Invoke(t.Context(), method, wrapperspb.String("x"), &wrapperspb.StringValue{}))
	}

	const requests = "test_edge_server_requests_total"
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, requests, "method", pairFirstMethod, "status", "OK"))
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, requests, "method", metricsint.OverflowMethod, "status", "OK"))
}

func TestDefaultMetricsInterceptor_MirrorsTheInterceptorMaxMethods(t *testing.T) {
	t.Parallel()

	require.Equal(t, metricsint.DefaultMaxMethods, grpcconfig.DefaultMetricsInterceptor().MaxMethods)
}
