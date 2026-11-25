// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

const interceptorName = "recovery"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

// ServerInterceptor returns a new interceptor that recovers from panics.
// It calls the configured PanicHandler when present. By default a panic
// becomes an Internal status.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		server: true,
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that recovers from panics.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that recovers from panics.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
}

// ClientInterceptor returns a new interceptor that recovers from panics in client calls.
// It calls the configured PanicHandler when present. By default a panic
// becomes an Internal status.
func ClientInterceptor(opt ...Option) interceptors.ClientInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}
}

// ClientUnaryInterceptor returns a new unary client interceptor that recovers from panics.
func ClientUnaryInterceptor(opt ...Option) stdGrpc.UnaryClientInterceptor {
	return ClientInterceptor(opt...).ClientUnaryInterceptor()
}

// ClientStreamInterceptor returns a new streaming client interceptor that recovers from panics.
func ClientStreamInterceptor(opt ...Option) stdGrpc.StreamClientInterceptor {
	return ClientInterceptor(opt...).ClientStreamInterceptor()
}

var _ interceptors.ServerInterceptor = (*interceptor)(nil)
var _ interceptors.ClientInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	opts   *options
	server bool
}

// Dependencies returns interceptors that recovery reads from context.
// All dependencies are optional for ordering - recovery gracefully degrades
// if requestid is not available in context.
func (i *interceptor) Dependencies() []string {
	if i.server {
		return []string{metadata.Name(), requestid.Name(), errstatus.Name()}
	}
	return []string{metadata.Name(), requestid.Name()}
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (_ any, err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(info.FullMethod) {
			return handler(ctx, req)
		}

		// Create a panicHandler that converts panic to gRPC error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContext(ctx, info.FullMethod, info)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = i.handlePanic(ctx, r, meta.FullyMethodName)
			}
		}

		defer recoverCall(ctx, grpcHandler)

		return handler(ctx, req)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) (err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(info.FullMethod) {
			return handler(srv, stream)
		}

		// Create a panicHandler that converts panic to gRPC error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureServerStreamInContext(srv, stream.Context(), info)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = i.handlePanic(ctx, r, meta.FullyMethodName)
			}
		}

		defer recoverCall(stream.Context(), grpcHandler)

		return handler(srv, stream)
	}
}

// handlePanic logs the recovery and returns a generic gRPC status.
func (i *interceptor) handlePanic(ctx context.Context, p any, method string) error {
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("request_id", requestid.FromContext(ctx)),
		slog.String("category", recovery.PanicCategory),
	}
	cause := recovery.ErrPanicRecovered
	if i.opts.logRawErrors {
		panicErr := recovery.NewPanicError(p, 1)
		cause = panicErr
		attrs = append(attrs,
			slog.String("panic", diagnostics.ErrorText(fmt.Sprint(p), diagnostics.DefaultMaxErrorLength)),
			slog.Any("stack_trace", panicErr.Frames),
		)
	}
	i.opts.logger.LogAttrs(ctx, slog.LevelError, "panic recovered", attrs...)
	return interceptors.NewError(status.New(codes.Internal, "Internal Error"), cause)
}

// ClientUnaryInterceptor returns a new unary client interceptor that recovers from panics.
func (i *interceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *stdGrpc.ClientConn,
		invoker stdGrpc.UnaryInvoker,
		opts ...stdGrpc.CallOption,
	) (err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(method) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		// Create a panicHandler that converts panic to error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContextFromMethod(ctx, method)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = i.handlePanic(ctx, r, meta.FullyMethodName)
			}
		}

		defer recoverCall(ctx, grpcHandler)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientStreamInterceptor returns a new streaming client interceptor that recovers from panics.
func (i *interceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stream stdGrpc.ClientStream, err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(method) {
			return streamer(ctx, desc, cc, method, opts...)
		}

		// Create a panicHandler that converts panic to error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContextFromMethod(ctx, method)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = i.handlePanic(ctx, r, meta.FullyMethodName)
			}
		}

		defer recoverCall(ctx, grpcHandler)

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// recoverCall handles RPC panics locally so process-wide hooks cannot
// log the raw panic or stack independently of the interceptor's settings.
func recoverCall(ctx context.Context, handle func(context.Context, any)) {
	if raw := recover(); raw != nil {
		handle(ctx, raw)
	}
}
