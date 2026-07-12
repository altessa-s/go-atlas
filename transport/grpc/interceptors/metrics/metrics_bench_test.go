// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

// BenchmarkServerUnaryInterceptor measures the per-request cost of the unary
// metrics interceptor against a real Prometheus-backed collector — the label
// handling between the interceptor and the adapter is exactly what this
// package's hot path is about.
func BenchmarkServerUnaryInterceptor(b *testing.B) {
	tc := testhelpers.NewTestCollector()
	info := &stdGrpc.UnaryServerInfo{FullMethod: "/bench.Service/Method"}

	b.Run("ok", func(b *testing.B) {
		intercept := ServerUnaryInterceptor(WithCollector(tc))
		handler := func(context.Context, any) (any, error) { return "ok", nil }
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, _ = intercept(ctx, "req", info, handler)
		}
	})

	b.Run("error", func(b *testing.B) {
		intercept := ServerUnaryInterceptor(WithCollector(tc))
		wantErr := status.Error(codes.InvalidArgument, "bad")
		handler := func(context.Context, any) (any, error) { return nil, wantErr }
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, _ = intercept(ctx, "req", info, handler)
		}
	})
}
