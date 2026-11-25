// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcstatus_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/internal/grpcstatus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BenchmarkUnaryConversion(b *testing.B) {
	intercept := grpcstatus.New(nil, false, nil).ClientUnaryInterceptor()
	cancelled := status.Error(codes.Canceled, "canceled")
	invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		return cancelled
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = intercept(b.Context(), "/example/Method", nil, nil, nil, invoke)
	}
}
