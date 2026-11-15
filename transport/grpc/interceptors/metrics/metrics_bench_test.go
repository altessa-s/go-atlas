// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

// BenchmarkServerUnaryInterceptor measures the per-request cost of the unary
// metrics interceptor against a real Prometheus-backed collector — the label
// handling between the interceptor and the adapter is exactly what this
// package's hot path is about.
func BenchmarkServerUnaryInterceptor(b *testing.B) {
	tc := metricstest.NewTestCollector()
	info := &stdGrpc.UnaryServerInfo{FullMethod: "/bench.Service/Method"}

	b.Run("ok", func(b *testing.B) {
		intercept := ServerUnaryInterceptor(WithMeter(tc))
		handler := func(context.Context, any) (any, error) { return "ok", nil }
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, _ = intercept(ctx, "req", info, handler)
		}
	})

	b.Run("error", func(b *testing.B) {
		intercept := ServerUnaryInterceptor(WithMeter(tc))
		wantErr := status.Error(codes.InvalidArgument, "bad")
		handler := func(context.Context, any) (any, error) { return nil, wantErr }
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, _ = intercept(ctx, "req", info, handler)
		}
	})
}

func BenchmarkGetStatusCode(b *testing.B) {
	err := status.Error(codes.NotFound, "not found")
	for b.Loop() {
		getStatusCode(err)
	}
}

// BenchmarkShouldSampleMessage measures the per-message sampling decision on
// the PerMessageSampling hot path.
func BenchmarkShouldSampleMessage(b *testing.B) {
	i := &interceptor{opts: newOptions(
		WithEnableStreamMetrics(),
		WithStreamSamplingRate(0.5),
		WithStreamSamplingStrategy(PerMessageSampling),
	)}
	s := &streamWrapper{interceptor: i, fullMethod: "/bench.Service/Stream"}
	s.initializeSampling()

	var n int64
	b.ReportAllocs()
	for b.Loop() {
		n++
		_ = s.shouldSampleMessage(n, true)
	}
}
