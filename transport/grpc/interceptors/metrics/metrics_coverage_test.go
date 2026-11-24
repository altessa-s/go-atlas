// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sizedMessage implements the Size() int sizing interface probed by
// getMessageSize, so request/response size metrics can be observed.
type sizedMessage struct{ n int }

func (m sizedMessage) Size() int { return m.n }

// fakeServerStream is a minimal grpc.ServerStream double that records the
// messages passed through SendMsg/RecvMsg.
type fakeServerStream struct {
	grpc.ServerStream
	ctx      context.Context
	sendMsgs []any
	recvMsgs []any
	recvErr  error
	sendErr  error
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func (f *fakeServerStream) SendMsg(m any) error {
	f.sendMsgs = append(f.sendMsgs, m)
	return f.sendErr
}

func (f *fakeServerStream) RecvMsg(m any) error {
	if f.recvErr != nil {
		return f.recvErr
	}
	f.recvMsgs = append(f.recvMsgs, m)
	return nil
}

func okUnaryHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

func TestServerUnaryInterceptor_RecordsSuccessMetrics(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc)).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Unary"}

	resp, err := inter(t.Context(), "req", info, okUnaryHandler)

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total",
		"method", "/test.Service/Unary", "status", "OK"))
	require.Equal(t, float64(0), metricstest.GetGaugeValue(t, tc, "test_grpc_server_requests_in_flight"))
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_grpc_server_request_duration_seconds",
		"method", "/test.Service/Unary", "status", "OK"))
}

func TestServerUnaryInterceptor_RecordsErrorStatus(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc)).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Unary"}
	handlerErr := status.Error(codes.NotFound, "missing")

	resp, err := inter(t.Context(), "req", info, func(context.Context, any) (any, error) {
		return nil, handlerErr
	})

	require.Nil(t, resp)
	require.ErrorIs(t, err, handlerErr)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total",
		"method", "/test.Service/Unary", "status", "NotFound"))
}

func TestServerUnaryInterceptor_SizeMetrics(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc), WithEnableSizeMetrics()).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Sized"}

	_, err := inter(t.Context(), sizedMessage{n: 42}, info, func(_ context.Context, req any) (any, error) {
		return sizedMessage{n: 7}, nil
	})

	require.NoError(t, err)
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_grpc_server_request_size_bytes",
		"method", "/test.Service/Sized", "status", "OK"))
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_grpc_server_response_size_bytes",
		"method", "/test.Service/Sized", "status", "OK"))
}

func TestServerStreamInterceptor_RecordsMetrics(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc)).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, func(_ any, s grpc.ServerStream) error {
		return s.SendMsg("hello")
	})

	require.NoError(t, err)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total",
		"method", "/test.Service/Stream", "status", "OK"))
	require.Equal(t, float64(0), metricstest.GetGaugeValue(t, tc, "test_grpc_server_requests_in_flight"))
	require.Len(t, stream.sendMsgs, 1)
}

func TestServerStreamInterceptor_PropagatesHandlerError(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc)).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}
	handlerErr := status.Error(codes.Unavailable, "down")

	err := inter(nil, stream, info, func(any, grpc.ServerStream) error { return handlerErr })

	require.ErrorIs(t, err, handlerErr)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_requests_total",
		"method", "/test.Service/Stream", "status", "Unavailable"))
}

// TestServerStreamInterceptor_StreamMetricsFullSampling exercises the
// deterministic sampling-rate boundaries (1.0 and 0.0), which never
// consult the random sampling decision, so counts are exact.
func TestServerStreamInterceptor_StreamMetricsFullSampling(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc), WithEnableStreamMetrics(),
		WithStreamSamplingRate(MaxStreamSamplingRate)).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, func(_ any, s grpc.ServerStream) error {
		require.NoError(t, s.SendMsg("out"))
		require.NoError(t, s.RecvMsg(new(string)))
		require.NoError(t, s.RecvMsg(new(string)))
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_sent_total",
		"method", "/test.Service/Stream", "status", "OK"))
	require.Equal(t, float64(2), metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_received_total",
		"method", "/test.Service/Stream", "status", "OK"))
}

func TestServerStreamMetricsExcludeFailedSend(t *testing.T) {
	t.Parallel()
	const method = "/test.Service/Stream"
	meter := metricstest.NewTestCollector()
	intercept := ServerStreamInterceptor(WithMeter(meter), WithEnableStreamMetrics(), WithEnableSizeMetrics())
	stream := &fakeServerStream{ctx: t.Context(), sendErr: status.Error(codes.Unavailable, "connection closed")}
	err := intercept(nil, stream, &grpc.StreamServerInfo{FullMethod: method}, func(_ any, s grpc.ServerStream) error {
		return s.SendMsg(sizedMessage{n: 42})
	})
	require.ErrorIs(t, err, stream.sendErr)
	require.Zero(t, metricstest.GetCounterValue(t, meter, "test_grpc_server_stream_messages_sent_total", "method", method, "status", "Unavailable"))
	require.Zero(t, metricstest.GetHistogramCount(t, meter, "test_grpc_server_stream_message_size_bytes", "method", method, "direction", "sent"))
}

// TestServerStreamInterceptor_StreamMetricsZeroSampling confirms that a
// sampling rate of 0 disables all stream message metrics deterministically.
func TestServerStreamInterceptor_StreamMetricsZeroSampling(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc), WithEnableStreamMetrics(),
		WithStreamSamplingRate(MinStreamSamplingRate)).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, func(_ any, s grpc.ServerStream) error {
		return s.SendMsg("out")
	})

	require.NoError(t, err)
	require.Equal(t, float64(0), metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_sent_total",
		"method", "/test.Service/Stream", "status", "OK"))
}

// TestServerStreamInterceptor_PerStreamSampling exercises the PerStreamSampling
// strategy at the deterministic full-sampling boundary.
func TestServerStreamInterceptor_PerStreamSampling(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	inter := ServerInterceptor(WithMeter(tc), WithEnableStreamMetrics(),
		WithStreamSamplingRate(MaxStreamSamplingRate),
		WithStreamSamplingStrategy(PerStreamSampling)).ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, func(_ any, s grpc.ServerStream) error {
		return s.SendMsg("out")
	})

	require.NoError(t, err)
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_sent_total",
		"method", "/test.Service/Stream", "status", "OK"))
}

func TestServerInterceptor_Name(t *testing.T) {
	t.Parallel()

	require.Equal(t, "metrics", Name())

	tc := metricstest.NewTestCollector()
	i := ServerInterceptor(WithMeter(tc))
	require.Equal(t, "metrics", i.Name())
}

func TestGetMessageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      any
		wantSize int
		wantOk   bool
	}{
		{"nil message", nil, 0, false},
		{"sizeable message", sizedMessage{n: 12}, 12, true},
		{"unsized message", "not sizeable", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			size, ok := getMessageSize(tc.msg)

			require.Equal(t, tc.wantOk, ok)
			require.Equal(t, tc.wantSize, size)
		})
	}
}

func TestWithDurationBuckets(t *testing.T) {
	t.Parallel()

	t.Run("valid buckets applied", func(t *testing.T) {
		t.Parallel()

		o := defaultOptions()
		WithDurationBuckets([]float64{1, 2, 3})(o)
		require.Equal(t, []float64{1, 2, 3}, o.durationBuckets)
	})

	t.Run("empty buckets is a no-op", func(t *testing.T) {
		t.Parallel()

		o := defaultOptions()
		original := o.durationBuckets
		WithDurationBuckets(nil)(o)
		require.Equal(t, original, o.durationBuckets)
	})

	t.Run("non-increasing buckets panics", func(t *testing.T) {
		t.Parallel()

		o := defaultOptions()
		require.Panics(t, func() { WithDurationBuckets([]float64{2, 1})(o) })
	})
}

func TestWithSizeBuckets(t *testing.T) {
	t.Parallel()

	t.Run("valid buckets applied", func(t *testing.T) {
		t.Parallel()

		o := defaultOptions()
		WithSizeBuckets([]float64{10, 20})(o)
		require.Equal(t, []float64{10, 20}, o.sizeBuckets)
	})

	t.Run("non-increasing buckets panics", func(t *testing.T) {
		t.Parallel()

		o := defaultOptions()
		require.Panics(t, func() { WithSizeBuckets([]float64{5, 5})(o) })
	})
}

func TestServerInterceptor_Dependencies_Coverage(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	i, ok := ServerInterceptor(WithMeter(tc)).(*serverInterceptorWrapper)
	require.True(t, ok)
	require.Equal(t, []string{"metadata"}, i.Dependencies())
}

func TestGetStatusCode_ContextErrors(t *testing.T) {
	t.Parallel()

	require.Equal(t, codes.DeadlineExceeded, getStatusCode(context.DeadlineExceeded))
	require.Equal(t, codes.Canceled, getStatusCode(context.Canceled))
	require.Equal(t, codes.Unknown, getStatusCode(errors.New("boom")))
}
