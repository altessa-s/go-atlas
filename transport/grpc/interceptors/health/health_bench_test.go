// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"

	"google.golang.org/grpc"
)

type nopHealth struct{}

func (nopHealth) Health(context.Context) error { return nil }

func benchHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

var benchErr error

// BenchmarkServerUnaryInterceptor_Healthy measures the per-request passthrough
// cost of the unary interceptor when the health check succeeds.
func BenchmarkServerUnaryInterceptor_Healthy(b *testing.B) {
	inter := health.ServerUnaryInterceptor(nopHealth{})
	ctx := b.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	b.ReportAllocs()
	for b.Loop() {
		_, benchErr = inter(ctx, nil, info, benchHandler)
	}
}

// BenchmarkServerUnaryInterceptor_IgnoredMethod measures the passthrough cost
// when the method is on the ignore list and the health check is skipped.
func BenchmarkServerUnaryInterceptor_IgnoredMethod(b *testing.B) {
	inter := health.ServerUnaryInterceptor(nopHealth{}, health.WithIgnoreMethods("/test.Service/Method"))
	ctx := b.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	b.ReportAllocs()
	for b.Loop() {
		_, benchErr = inter(ctx, nil, info, benchHandler)
	}
}
