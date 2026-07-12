// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
)

var (
	benchResp any
	benchErr  error
)

// benchServerUnary drives the unary interceptor with a handler that always
// returns handlerErr, measuring the error-to-status conversion path.
func benchServerUnary(b *testing.B, handlerErr error) {
	b.Helper()

	unary := ServerInterceptor().ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handler := func(ctx context.Context, req any) (any, error) { return nil, handlerErr }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, "req", info, handler)
	}
}

// BenchmarkServerUnaryInterceptor_PlainError measures conversion of a plain
// error without a gRPC status into the default codes.Internal status.
func BenchmarkServerUnaryInterceptor_PlainError(b *testing.B) {
	benchServerUnary(b, errors.New("boom"))
}

// BenchmarkServerUnaryInterceptor_SentinelError measures the built-in sentinel
// mapping path (context.DeadlineExceeded to codes.DeadlineExceeded) with the
// conversion cache warm after the first request.
func BenchmarkServerUnaryInterceptor_SentinelError(b *testing.B) {
	benchServerUnary(b, context.DeadlineExceeded)
}
