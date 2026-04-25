// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func BenchmarkNoOpInterceptor_ServerUnary(b *testing.B) {
	i := &NoOpInterceptor{}
	interceptor := i.ServerUnaryInterceptor()
	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	ctx := b.Context()
	info := &grpc.UnaryServerInfo{}
	for b.Loop() {
		interceptor(ctx, nil, info, handler) //nolint:errcheck
	}
}

func BenchmarkNoopDriver(b *testing.B) {
	d := NoopDriver()
	ctx := b.Context()
	for b.Loop() {
		d.PreCall(ctx, nil)       //nolint:errcheck
		d.PostCall(ctx, nil, nil) //nolint:errcheck
	}
}
