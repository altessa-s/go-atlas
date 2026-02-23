// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

var _ ServerInterceptor = (*NoOpInterceptor)(nil)

// NoOpInterceptor provides pass-through server interception.
type NoOpInterceptor struct{}

// ServerUnaryInterceptor returns pass-through unary interceptor.
func (i *NoOpInterceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ any, err error) {
		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns pass-through stream interceptor.
func (i *NoOpInterceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		return handler(srv, stream)
	}
}

// Name returns "noop". It satisfies [Interceptor] so a [NoOpInterceptor]
// can participate in [Chain] dependency ordering without affecting requests.
func (i *NoOpInterceptor) Name() string {
	return "noop"
}

var _ ClientInterceptor = (*NoOpClientInterceptor)(nil)

// NoOpClientInterceptor provides pass-through client interception.
// It delegates every call directly to the underlying invoker or streamer
// without modification, useful as a default when no client-side
// interception is needed.
type NoOpClientInterceptor struct{}

// Name returns "noop". It satisfies [Interceptor] so a
// [NoOpClientInterceptor] can participate in [Chain] dependency ordering
// without affecting requests.
func (i *NoOpClientInterceptor) Name() string {
	return "noop"
}

// ClientUnaryInterceptor returns pass-through unary interceptor.
func (i *NoOpClientInterceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientStreamInterceptor returns pass-through stream interceptor.
func (i *NoOpClientInterceptor) ClientStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// NoopDriver returns a no-operation driver for testing and defaults.
//
// Example:
//
//	driver := interceptors.NoopDriver()
//	// Use where Driver is expected
func NoopDriver() driver.Driver {
	return driver.NoopDriver()
}
