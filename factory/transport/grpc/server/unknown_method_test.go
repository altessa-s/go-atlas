// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/transport/grpc/server"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
)

func TestUnknownMethodsShareTelemetryLabels(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Metrics: &grpcconfig.MetricsInterceptor{BaseInterceptor: on, Subsystem: "rpc"},
		Tracing: &grpcconfig.TracingInterceptor{BaseInterceptor: on},
	}}
	collector := metricstest.NewTestCollector()
	capture := statusCapture{adapters.NewMultiAdapter(), make(chan adapters.SpanData, 3)}
	tracer := tracing.New(tracing.WithAdapter(capture))
	t.Cleanup(func() { require.NoError(t, tracer.Shutdown(context.WithoutCancel(t.Context()))) })

	unknownHandler := grpc.UnknownServiceHandler(func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.Unimplemented, "unregistered method")
	})
	b := factory.New(cfg).UseMeter(collector).UseTracer(tracer).
		UseServerOptions(grpcserver.WithGrpcOptions(unknownHandler)).WithInterceptors()
	conn, _ := startBuilt(t, b)

	_, err := echo(t.Context(), conn, "known call")
	require.NoError(t, err)
	for _, method := range []string{"/unregistered.Service/Call", "/cov.v1.Echo/Unregistered"} {
		err := conn.Invoke(t.Context(), method, wrapperspb.String("unknown call"), &wrapperspb.StringValue{})
		require.Equal(t, codes.Unimplemented, status.Code(err))
	}

	require.Equal(t, 2.0, metricstest.GetCounterValue(t, collector, "test_rpc_server_requests_total",
		"method", metadata.UnknownMethod, "status", codes.Unimplemented.String()))
	require.NoError(t, tracer.ForceFlush(t.Context()))
	require.Len(t, capture.spans, 3)

	names := make([]string, 0, 3)
	for range 3 {
		names = append(names, (<-capture.spans).Name)
	}
	require.ElementsMatch(t, []string{covMethod, metadata.UnknownMethod, metadata.UnknownMethod}, names)
}
