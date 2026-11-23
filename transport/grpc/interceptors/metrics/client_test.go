// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"io"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClientUnaryMetrics(t *testing.T) {
	t.Parallel()

	const method = "/test.Service/Call"
	tc := metricstest.NewTestCollector()
	intercept := ClientUnaryInterceptor(WithMeter(tc))
	for _, tt := range []struct {
		code codes.Code
		err  error
	}{
		{codes.OK, nil},
		{codes.Unavailable, status.Error(codes.Unavailable, "unavailable")},
		{codes.DeadlineExceeded, context.DeadlineExceeded},
		{codes.Canceled, context.Canceled},
	} {
		t.Run(tt.code.String(), func(t *testing.T) {
			err := intercept(t.Context(), method, nil, nil, nil,
				func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
					require.Equal(t, 1.0, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
					require.Equal(t, 1.0, metricstest.GetGaugeValue(t, tc,
						"test_grpc_client_requests_in_flight_by_method", "method", method))
					return tt.err
				})
			require.ErrorIs(t, err, tt.err)
			require.Zero(t, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
			require.Zero(t, metricstest.GetGaugeValue(t, tc,
				"test_grpc_client_requests_in_flight_by_method", "method", method))
			assertClientRequest(t, tc, method, tt.code, 1)
		})
	}
	require.False(t, tc.Exists("test_grpc_client_request_size_bytes"))
	require.False(t, tc.Exists("test_grpc_client_stream_messages_sent_total"))
}

type metricsClientStream struct {
	grpc.ClientStream
	finish func(error)
}

func (s metricsClientStream) SendMsg(any) error { return nil }
func (s metricsClientStream) CloseSend() error  { return nil }
func (s metricsClientStream) RecvMsg(m any) error {
	if m == nil {
		s.finish(nil)
		return io.EOF
	}
	if _, ok := m.(finalClientSizedMessage); ok {
		s.finish(nil)
	}
	return nil
}

type clientSizedMessage struct{}

func (clientSizedMessage) Size() int { return 42 }

func TestClientUnarySizeMetrics(t *testing.T) {
	t.Parallel()

	const method = "/test.Service/Call"
	tc := metricstest.NewTestCollector()
	intercept := ClientUnaryInterceptor(WithMeter(tc), WithEnableSizeMetrics())
	for _, code := range []codes.Code{codes.OK, codes.Unavailable} {
		failure := status.Error(code, "call status")
		msg := clientSizedMessage{}
		err := intercept(t.Context(), method, msg, msg, nil,
			func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return failure })
		require.ErrorIs(t, err, failure)
		require.EqualValues(t, 1, metricstest.GetHistogramCount(t, tc,
			"test_grpc_client_request_size_bytes", "method", method, "status", code.String()))
	}
	require.EqualValues(t, 1, metricstest.GetHistogramCount(t, tc,
		"test_grpc_client_response_size_bytes", "method", method, "status", "OK"))
	require.Zero(t, metricstest.GetHistogramCount(t, tc,
		"test_grpc_client_response_size_bytes", "method", method, "status", "Unavailable"))
}

func TestClientStreamMetrics(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		terminalRecv bool
	}{
		{name: "server streaming"},
		{name: "client streaming final response", terminalRecv: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const method = "/test.Service/Stream"
			tc := metricstest.NewTestCollector()
			intercept := ClientInterceptor(WithMeter(tc), WithEnableSizeMetrics(), WithEnableStreamMetrics())
			stream, err := intercept.ClientStreamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, method,
				func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					finish, ok := opts[len(opts)-1].(grpc.OnFinishCallOption)
					require.True(t, ok)
					return metricsClientStream{finish: finish.OnFinish}, nil
				})
			require.NoError(t, err)
			require.Equal(t, 1.0, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
			require.NoError(t, stream.SendMsg(clientSizedMessage{}))
			require.NoError(t, stream.CloseSend())
			require.Equal(t, 1.0, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
			if tt.terminalRecv {
				require.NoError(t, stream.RecvMsg(finalClientSizedMessage{}))
			} else {
				require.NoError(t, stream.RecvMsg(clientSizedMessage{}))
				require.ErrorIs(t, stream.RecvMsg(nil), io.EOF)
			}
			require.Zero(t, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
			assertClientRequest(t, tc, method, codes.OK, 1)
			for _, direction := range []string{"sent", "received"} {
				require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc,
					"test_grpc_client_stream_messages_"+direction+"_total", "method", method, "status", "OK"))
				require.EqualValues(t, 1, metricstest.GetHistogramCount(t, tc,
					"test_grpc_client_stream_message_size_bytes", "method", method, "direction", direction))
			}
		})
	}
}

type finalClientSizedMessage struct{ clientSizedMessage }

func TestClientStreamMetricsStartFailure(t *testing.T) {
	t.Parallel()

	const method = "/test.Service/Stream"
	tc := metricstest.NewTestCollector()
	failure := status.Error(codes.Unavailable, "unavailable")
	_, err := ClientStreamInterceptor(WithMeter(tc))(t.Context(), &grpc.StreamDesc{}, nil, method,
		func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			finish, ok := opts[len(opts)-1].(grpc.OnFinishCallOption)
			require.True(t, ok)
			finish.OnFinish(failure)
			return nil, failure
		})
	require.ErrorIs(t, err, failure)
	require.Zero(t, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
	assertClientRequest(t, tc, method, codes.Unavailable, 1)
}

func TestClientMetricsMethodCardinality(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	intercept := ClientInterceptor(WithMeter(tc), WithMaxMethods(1))
	unary := intercept.ClientUnaryInterceptor()
	invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return nil }
	for _, method := range []string{"/test.Service/First", "/test.Service/Second", "/test.Service/Third", "/test.Service/First"} {
		require.NoError(t, unary(t.Context(), method, nil, nil, nil, invoke))
	}
	assertClientRequest(t, tc, "/test.Service/First", codes.OK, 2)
	assertClientRequest(t, tc, OverflowMethod, codes.OK, 2)
	assertClientRequest(t, tc, "/test.Service/Second", codes.OK, 0)
	assertClientRequest(t, tc, "/test.Service/Third", codes.OK, 0)
}

func TestClientMetricsIgnoredMethods(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		options []Option
		methods []string
	}{
		{name: "defaults", methods: []string{
			"/grpc.health.v1.Health/Check",
			"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
		}},
		{name: "custom", options: []Option{
			WithIgnoreMethods("/test.Service/Exact"),
			WithIgnorePatterns(regexp.MustCompile(`^/test\.ignored/`)),
		}, methods: []string{"/test.Service/Exact", "/test.Ignored/Other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := metricstest.NewTestCollector()
			intercept := ClientInterceptor(append(tt.options, WithMeter(tc), WithMaxMethods(1))...)
			unary, stream := intercept.ClientUnaryInterceptor(), intercept.ClientStreamInterceptor()
			invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				require.Zero(t, tc.GaugeValue("test_grpc_client_requests_in_flight", nil))
				return nil
			}
			for _, method := range tt.methods {
				require.NoError(t, unary(t.Context(), method, nil, nil, nil, invoke))
				_, err := stream(t.Context(), &grpc.StreamDesc{}, nil, method,
					func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
						require.Empty(t, opts)
						return metricsClientStream{}, nil
					})
				require.NoError(t, err)
				assertClientRequest(t, tc, method, codes.OK, 0)
			}
			const tracked = "/test.Service/Tracked"
			require.NoError(t, unary(t.Context(), tracked, nil, nil, nil,
				func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return nil }))
			assertClientRequest(t, tc, tracked, codes.OK, 1)
			assertClientRequest(t, tc, OverflowMethod, codes.OK, 0)
		})
	}
}

func assertClientRequest(t *testing.T, tc *metricstest.TestCollector, method string, code codes.Code, count int) {
	t.Helper()
	require.Equal(t, float64(count), metricstest.GetCounterValue(t, tc,
		"test_grpc_client_requests_total", "method", method, "status", code.String()))
	require.EqualValues(t, count, metricstest.GetHistogramCount(t, tc,
		"test_grpc_client_request_duration_seconds", "method", method, "status", code.String()))
}

func TestConcurrentFirstCallsShareMethodLabel(t *testing.T) {
	const method = "/test.Service/Call"
	const calls = 32
	meter := metricstest.NewTestCollector()
	intercept := ClientUnaryInterceptor(WithMeter(meter), WithMaxMethods(1))
	start := make(chan struct{})
	var group sync.WaitGroup
	for range calls {
		group.Go(func() {
			<-start
			err := intercept(t.Context(), method, nil, nil, nil,
				func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return nil })
			if err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	group.Wait()
	assertClientRequest(t, meter, method, codes.OK, calls)
	assertClientRequest(t, meter, OverflowMethod, codes.OK, 0)
}
