// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
)

// benchDriver implements every stream hook as a pass-through, so the
// benchmarks measure wrapper dispatch rather than hook work.
type benchDriver struct{}

func (benchDriver) PreCall(context.Context, any) (any, error)          { return nil, nil } //nolint:nilnil // proceed
func (benchDriver) PostCall(_ context.Context, _ any, err error) error { return err }
func (benchDriver) PostMsgReceive(_ context.Context, _ any, err error) error {
	return err
}
func (benchDriver) PostMsgSent(_ context.Context, _ any, err error) error { return err }
func (benchDriver) PreMsgSend(_ context.Context, m any) (any, error)      { return m, nil }

// chainedServerStream wraps a mock stream with one nil-driver wrapper (as
// auth, realip, requestid and tracing add) and drivers driven interceptors.
func chainedServerStream(ctx context.Context, drivers int) grpc.ServerStream {
	var ss grpc.ServerStream = NewServerWrappedStream(ctx, &mockServerStream{ctx: ctx}, nil)
	for range drivers {
		ss = NewServerWrappedStream(ctx, ss, benchDriver{})
	}
	return ss
}

// BenchmarkServerStreamWrapper_SendMsg measures a send through chains of
// stream wrappers; each driver beyond the first adds one nested wrapper.
func BenchmarkServerStreamWrapper_SendMsg(b *testing.B) {
	for _, drivers := range []int{0, 1, 3} {
		b.Run(fmt.Sprintf("drivers=%d", drivers), func(b *testing.B) {
			ss := chainedServerStream(b.Context(), drivers)
			b.ReportAllocs()
			for b.Loop() {
				_ = ss.SendMsg("msg")
			}
		})
	}
}

// BenchmarkServerStreamWrapper_RecvMsg measures a receive through chains of
// stream wrappers.
func BenchmarkServerStreamWrapper_RecvMsg(b *testing.B) {
	for _, drivers := range []int{0, 1, 3} {
		b.Run(fmt.Sprintf("drivers=%d", drivers), func(b *testing.B) {
			ss := chainedServerStream(b.Context(), drivers)
			b.ReportAllocs()
			for b.Loop() {
				_ = ss.RecvMsg(nil)
			}
		})
	}
}

// BenchmarkNewServerWrappedStream measures building the wrapper chain once
// per stream, the allocation cost of nesting.
func BenchmarkNewServerWrappedStream(b *testing.B) {
	for _, drivers := range []int{0, 1, 3} {
		b.Run(fmt.Sprintf("drivers=%d", drivers), func(b *testing.B) {
			ctx := b.Context()
			b.ReportAllocs()
			for b.Loop() {
				_ = chainedServerStream(ctx, drivers)
			}
		})
	}
}
