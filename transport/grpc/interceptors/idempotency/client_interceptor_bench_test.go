// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func benchNoopInvoker(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
	return nil
}

func BenchmarkUnaryClientInterceptor_Stamps(b *testing.B) {
	interceptor := idempotency.UnaryClientInterceptor()
	ctx := idempotency.WithOperation(context.Background(), "op-42")

	b.ReportAllocs()
	for b.Loop() {
		_ = interceptor(ctx, "/svc.v1.Users/Update", nil, nil, nil, benchNoopInvoker)
	}
}

func BenchmarkUnaryClientInterceptor_NoSeed(b *testing.B) {
	interceptor := idempotency.UnaryClientInterceptor()
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_ = interceptor(ctx, "/svc.v1.Users/Update", nil, nil, nil, benchNoopInvoker)
	}
}

func BenchmarkUnaryClientInterceptor_ExplicitKey(b *testing.B) {
	interceptor := idempotency.UnaryClientInterceptor()
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		idempotency.DefaultIdempotencyKeyHeader, "11111111-1111-4111-8111-111111111111",
	)
	ctx = idempotency.WithOperation(ctx, "op-42")

	b.ReportAllocs()
	for b.Loop() {
		_ = interceptor(ctx, "/svc.v1.Users/Update", nil, nil, nil, benchNoopInvoker)
	}
}
