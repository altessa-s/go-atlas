// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

var (
	benchResp any
	benchErr  error
)

// BenchmarkServerUnaryInterceptor_NoPanic measures the passthrough cost of the
// recovery interceptor when the handler returns normally — the price every
// request pays for the deferred panic guard.
func BenchmarkServerUnaryInterceptor_NoPanic(b *testing.B) {
	unary := ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handler := func(ctx context.Context, req any) (any, error) { return req, nil }
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, "req", info, handler)
	}
}
