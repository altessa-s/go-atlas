// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc"

	grpcmetrics "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
)

const streamMethod = "/svc.Feed/Watch"

type nopStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *nopStream) Context() context.Context { return s.ctx }
func (s *nopStream) SendMsg(any) error        { return nil }
func (s *nopStream) RecvMsg(any) error        { return nil }

func sendN(n int) grpc.StreamHandler {
	return func(_ any, s grpc.ServerStream) error {
		for range n {
			if err := s.SendMsg("m"); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "metrics", grpcmetrics.Name())
	require.Equal(t, "metrics", grpcmetrics.ID.Name())
}

// With a fractional rate the recorded total is the sampled count scaled by
// 1/rate, so it is always a whole multiple of that factor and never exceeds
// the scaled number of messages sent.
func TestStreamSampling_FractionalRateScalesTotals(t *testing.T) {
	t.Parallel()

	const (
		messages = 50
		rate     = 0.5
	)
	for _, strategy := range []grpcmetrics.StreamSamplingStrategy{grpcmetrics.PerMessageSampling, grpcmetrics.PerStreamSampling} {
		t.Run(string(strategy), func(t *testing.T) {
			t.Parallel()
			tc := metricstest.NewTestCollector()
			stream := grpcmetrics.ServerStreamInterceptor(
				grpcmetrics.WithMeter(tc),
				grpcmetrics.WithEnableStreamMetrics(),
				grpcmetrics.WithEnableSizeMetrics(),
				grpcmetrics.WithStreamSamplingRate(rate),
				grpcmetrics.WithStreamSamplingStrategy(strategy),
			)
			require.NoError(t, stream(nil, &nopStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: streamMethod}, sendN(messages)))

			got := metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_sent_total", "method", streamMethod, "status", "OK")
			require.LessOrEqual(t, got, float64(messages)/rate)
			require.Zero(t, math.Mod(got, 1/rate), "total %v must be a multiple of the scale factor", got)
			if strategy == grpcmetrics.PerStreamSampling {
				require.Contains(t, []float64{0, float64(messages) / rate}, got, "a stream is sampled as a unit")
			}
			require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_grpc_server_request_duration_seconds",
				"method", streamMethod, "status", "OK"))
		})
	}
}

func TestIgnoredMethodsAreNotMeasured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opt  grpcmetrics.Option
	}{
		{name: "method", opt: grpcmetrics.WithIgnoreMethods(" /svc.Feed/Get ")},
		{name: "pattern", opt: grpcmetrics.WithIgnorePatterns(regexp.MustCompile(`^/svc\.feed/`))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			col := metricstest.NewTestCollector()
			opts := []grpcmetrics.Option{
				grpcmetrics.WithMeter(col), tc.opt,
				grpcmetrics.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
			}

			resp, err := grpcmetrics.ServerUnaryInterceptor(opts...)(t.Context(), "req",
				&grpc.UnaryServerInfo{FullMethod: "/svc.Feed/Get"},
				func(context.Context, any) (any, error) { return "ok", nil })
			require.NoError(t, err)
			require.Equal(t, "ok", resp)

			if tc.name == "pattern" {
				require.NoError(t, grpcmetrics.ServerStreamInterceptor(opts...)(nil, &nopStream{ctx: t.Context()},
					&grpc.StreamServerInfo{FullMethod: streamMethod}, sendN(1)))
			}
			require.Zero(t, metricstest.GetCounterValue(t, col, "test_grpc_server_requests_total", "method", "/svc.Feed/Get", "status", "OK"))
			require.Zero(t, metricstest.GetCounterValue(t, col, "test_grpc_server_requests_total", "method", streamMethod, "status", "OK"))
			require.Contains(t, logs.String(), "ignored")
		})
	}
}

func TestMetricsSubsystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opt  grpcmetrics.Option
		want string
	}{
		{name: "string", opt: grpcmetrics.WithMetricsSubsystem("admin"), want: "test_admin_server_requests_total"},
		{name: "pointer", opt: grpcmetrics.WithMetricsSubsystem(new(" public ")), want: "test_public_server_requests_total"},
		{name: "nil pointer keeps default", opt: grpcmetrics.WithMetricsSubsystem((*string)(nil)), want: "test_grpc_server_requests_total"},
		{name: "blank keeps default", opt: grpcmetrics.WithMetricsSubsystem("  "), want: "test_grpc_server_requests_total"},
		{name: "blank pointer keeps default", opt: grpcmetrics.WithMetricsSubsystem(new("")), want: "test_grpc_server_requests_total"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			col := metricstest.NewTestCollector()
			_, err := grpcmetrics.ServerUnaryInterceptor(grpcmetrics.WithMeter(col), tc.opt)(t.Context(), "req",
				&grpc.UnaryServerInfo{FullMethod: "/svc.A/Get"},
				func(context.Context, any) (any, error) { return "ok", nil })
			require.NoError(t, err)
			require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, tc.want, "method", "/svc.A/Get", "status", "OK"))
		})
	}
}

// PerMessageSampling must decide for each message independently: at rate 0.5
// most streams record a strict subset of their messages and the overall
// recorded fraction stays close to the rate.
func TestStreamSampling_PerMessageDecidesPerMessage(t *testing.T) {
	t.Parallel()

	const (
		streams  = 200
		messages = 64
		rate     = 0.5
	)
	var sampled, partial int
	for range streams {
		tc := metricstest.NewTestCollector()
		stream := grpcmetrics.ServerStreamInterceptor(
			grpcmetrics.WithMeter(tc),
			grpcmetrics.WithEnableStreamMetrics(),
			grpcmetrics.WithStreamSamplingRate(rate),
			grpcmetrics.WithStreamSamplingStrategy(grpcmetrics.PerMessageSampling),
		)
		require.NoError(t, stream(nil, &nopStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: streamMethod}, sendN(messages)))

		got := int(metricstest.GetCounterValue(t, tc, "test_grpc_server_stream_messages_sent_total",
			"method", streamMethod, "status", "OK") * rate)
		sampled += got
		if got > 0 && got < messages {
			partial++
		}
	}

	fraction := float64(sampled) / float64(streams*messages)
	require.InDelta(t, rate, fraction, 0.05, "recorded fraction %v must track the sampling rate", fraction)
	require.Greater(t, partial, streams*9/10, "streams must not be sampled all-or-nothing (%d/%d partial)", partial, streams)
}
