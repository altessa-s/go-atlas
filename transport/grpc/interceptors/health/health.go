// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	stdGrpc "google.golang.org/grpc"
)

// Health defines the contract for service health checking.
// Implementations should perform a fast, non-blocking check of critical
// dependencies (databases, caches, downstream services) and return nil
// if the service can process requests, or an error describing the failure.
//
// Returning an error causes the interceptor to reject the request with
// [codes.Unavailable]. If the returned error is already a gRPC status,
// it is forwarded as-is.
type Health interface {
	Health(context.Context) error
}

var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	health Health
	opts   *options
}

// ErrServiceUnavailable is a sentinel error indicating that the service cannot
// handle requests. It is wrapped into the gRPC response by [ServerInterceptor],
// [ServerUnaryInterceptor], and [ServerStreamInterceptor] when the [Health]
// check returns a non-gRPC error. Callers can test for it with [errors.Is].
var ErrServiceUnavailable = errors.New("service unavailable")

// ServerInterceptor returns a new interceptor that checks the health of the service.
func ServerInterceptor(health Health, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"health",
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		health: health,
		opts:   opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that checks the health of the service.
func ServerUnaryInterceptor(health Health, opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(health, opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that checks the health of the service.
func ServerStreamInterceptor(health Health, opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(health, opt...).ServerStreamInterceptor()
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		method := i.InternMethod(info.FullMethod)

		// Skip health check for configured methods
		if i.ShouldIgnore(method) {
			i.LogIgnored(ctx, method)
			return handler(ctx, req)
		}

		i.LogDebug(ctx, "performing health check", method)
		if err := i.health.Health(ctx); err != nil {
			i.LogWarn(ctx, "health check failed", method, err)
			return nil, i.unavailableError(err)
		}
		i.LogDebug(ctx, "health check passed", method)
		return handler(ctx, req)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		ctx := stream.Context()
		method := i.InternMethod(info.FullMethod)

		// Skip health check for configured methods
		if i.ShouldIgnore(method) {
			i.LogIgnored(ctx, method)
			return handler(srv, stream)
		}

		i.LogDebug(ctx, "performing health check", method)
		if err := i.health.Health(ctx); err != nil {
			i.LogWarn(ctx, "health check failed", method, err)
			return i.unavailableError(err)
		}
		i.LogDebug(ctx, "health check passed", method)
		return handler(srv, stream)
	}
}

func (i *interceptor) unavailableError(err error) error {
	if err == nil {
		return nil
	}

	if st, ok := status.FromError(err); ok {
		return st.Err()
	}

	wrappedErr := err
	if !errors.Is(err, ErrServiceUnavailable) {
		wrappedErr = coreerrs.Wrapf(ErrServiceUnavailable, "%v", err)
	}

	return interceptors.NewError(status.New(codes.Unavailable, "Service Unavailable"), wrappedErr)
}
