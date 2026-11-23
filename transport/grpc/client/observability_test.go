// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/transport/grpc/client"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type observingBackend struct {
	grpc_testing.UnimplementedTestServiceServer
	unary  chan []string
	stream chan []string
}

func (b observingBackend) UnaryCall(ctx context.Context, _ *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
	b.unary <- metadata.ValueFromIncomingContext(ctx, "traceparent")
	if len(b.unary) == 1 {
		return nil, status.Error(codes.Unavailable, "retry")
	}
	return &grpc_testing.SimpleResponse{}, nil
}

func (b observingBackend) StreamingOutputCall(
	_ *grpc_testing.StreamingOutputCallRequest, stream grpc.ServerStreamingServer[grpc_testing.StreamingOutputCallResponse],
) error {
	b.stream <- metadata.ValueFromIncomingContext(stream.Context(), "traceparent")
	return stream.Send(&grpc_testing.StreamingOutputCallResponse{})
}

type spanCapture struct {
	*adapters.MultiAdapter
	spans chan adapters.SpanData
}

func (a spanCapture) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	for _, span := range spans {
		a.spans <- span
	}
	return nil
}

func TestClientObservability(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		enabled bool
	}{{"enabled", true}, {"disabled", false}} {
		t.Run(tt.name, func(t *testing.T) {
			lis := bufconn.Listen(1 << 20)
			backend := observingBackend{unary: make(chan []string, 2), stream: make(chan []string, 1)}
			srv := grpc.NewServer()
			grpc_testing.RegisterTestServiceServer(srv, backend)
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)

			tc := metricstest.NewTestCollector()
			capture := spanCapture{MultiAdapter: adapters.NewMultiAdapter(), spans: make(chan adapters.SpanData, 4)}
			tracer := tracing.New(tracing.WithAdapter(capture))
			var traceparentCount int
			if tt.enabled {
				traceparentCount = 1
			}
			var unaryCalls, streamCalls int
			var logs bytes.Buffer
			domainErr := errors.New("application error")
			opts := []client.Option{
				client.WithInsecure(), client.WithoutHealthCheck(),
				client.WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
				client.WithErrorConverter(func(context.Context, *status.Status) error { return domainErr }),
				client.WithRetry(client.WithRetryMaxAttempts(2)),
				client.WithDialOptions(
					grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
					grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
						invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
					) error {
						unaryCalls++
						require.Len(t, outgoingTraceparent(ctx), traceparentCount)
						return invoker(ctx, method, req, reply, cc, opts...)
					}),
					grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
						method string, streamer grpc.Streamer, opts ...grpc.CallOption,
					) (grpc.ClientStream, error) {
						streamCalls++
						require.Len(t, outgoingTraceparent(ctx), traceparentCount)
						return streamer(ctx, desc, cc, method, opts...)
					}),
				),
			}
			if tt.enabled {
				opts = append(opts, client.WithMeter(tc), client.WithTracer(tracer))
			}
			c, err := client.New(t.Context(), "passthrough:///bufnet", opts...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close(t.Context()) })
			conn, err := c.GetConnection(t.Context())
			require.NoError(t, err)
			rpc := grpc_testing.NewTestServiceClient(conn)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err = rpc.UnaryCall(ctx, &grpc_testing.SimpleRequest{})
			require.NoError(t, err)
			require.Equal(t, 1, unaryCalls)

			stream, err := rpc.StreamingOutputCall(ctx, &grpc_testing.StreamingOutputCallRequest{})
			require.NoError(t, err)
			_, err = stream.Recv()
			require.NoError(t, err)
			_, err = stream.Recv()
			require.ErrorIs(t, err, io.EOF)
			require.ErrorIs(t, stream.Context().Err(), context.Canceled)
			require.NoError(t, ctx.Err())
			require.Equal(t, 1, streamCalls)

			_, err = rpc.EmptyCall(ctx, &grpc_testing.Empty{})
			require.ErrorIs(t, err, domainErr)
			require.Contains(t, logs.String(), `"code":"Unimplemented"`)
			require.NotContains(t, logs.String(), "0001-01-01")

			require.NoError(t, tracer.Shutdown(t.Context()))
			require.Len(t, backend.unary, 2)
			require.Len(t, backend.stream, 1)
			const counter = "test_grpc_client_requests_total"
			if !tt.enabled {
				require.Empty(t, <-backend.unary)
				require.Empty(t, <-backend.unary)
				require.Empty(t, <-backend.stream)
				require.Empty(t, capture.spans)
				require.False(t, tc.Exists(counter))
				return
			}

			require.Len(t, capture.spans, 3)
			unarySpan, streamSpan := <-capture.spans, <-capture.spans
			require.Equal(t, traceparent(unarySpan), <-backend.unary)
			require.Equal(t, traceparent(unarySpan), <-backend.unary)
			require.Equal(t, traceparent(streamSpan), <-backend.stream)
			errorSpan := <-capture.spans
			require.Equal(t, adapters.StatusError, errorSpan.Status)
			require.Contains(t, errorSpan.Attributes, adapters.Attribute{Key: "rpc.grpc.status_code", Value: int(codes.Unimplemented)})
			require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, counter, "method", grpc_testing.TestService_EmptyCall_FullMethodName, "status", "Unimplemented"))
			for _, method := range []string{grpc_testing.TestService_UnaryCall_FullMethodName, grpc_testing.TestService_StreamingOutputCall_FullMethodName} {
				require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, counter, "method", method, "status", "OK"))
			}
		})
	}
}

func outgoingTraceparent(ctx context.Context) []string {
	md, _ := metadata.FromOutgoingContext(ctx)
	return md.Get("traceparent")
}

func traceparent(span adapters.SpanData) []string {
	return []string{"00-" + span.TraceID.String() + "-" + span.SpanID.String() + "-01"}
}
