// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/runtime/helpers"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/recovery"

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
// When panic occurs, it will call the provided panic panicHandler.
// If the panic panicHandler is not provided, it will use the default panic panicHandler.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
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

// ServerUnaryInterceptor returns a new unary server interceptor that recovers from panics.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that recovers from panics.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
}

// ClientInterceptor returns a new interceptor that recovers from panics in client calls.
// When panic occurs in client interceptors or during client call processing, it will call the provided panic panicHandler.
// If the panic panicHandler is not provided, it will use the default panic panicHandler.
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
	opts *options
}

// Dependencies returns interceptors that recovery reads from context.
// All dependencies are optional for ordering - recovery gracefully degrades
// if requestid is not available in context.
func (i *interceptor) Dependencies() []string {
	return []string{metadata.Name(), requestid.Name()}
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (_ any, err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(info.FullMethod) {
			return handler(ctx, req)
		}

		// If using panics package integration
		opts := panics.NewHandleOpts().SetReallyPanic(false)

		// Create a panicHandler that converts panic to gRPC error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContext(ctx, info.FullMethod, info)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = panicHandler(ctx, r, i.opts.logger, meta.FullyMethodName)
			}
		}

		defer panics.HandleWithOpts(ctx, opts, grpcHandler)

		return handler(ctx, req)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) (err error) {
		// Check if this method should skip recovery
		if i.ShouldIgnore(info.FullMethod) {
			return handler(srv, stream)
		}

		// If using panics package integration
		opts := panics.NewHandleOpts().SetReallyPanic(false)

		// Create a panicHandler that converts panic to gRPC error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContext(stream.Context(), info.FullMethod, info)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = panicHandler(ctx, r, i.opts.logger, meta.FullyMethodName)
			}
		}

		defer panics.HandleWithOpts(stream.Context(), opts, grpcHandler)

		return handler(srv, stream)
	}
}

// panicHandler is the default panic panicHandler that logs the panic internally
// and returns a generic error to prevent information disclosure.
func panicHandler(ctx context.Context, p any, logger *slog.Logger, method string) error {
	frames := recovery.StackTrace(1)

	// Get request ID if available
	requestID := requestid.FromContext(ctx)

	// Log the panic details internally
	logger.Error("panic recovered",
		slog.String("method", method),
		slog.String("request_id", requestID),
		slog.Any("panic", p),
		slog.Any("stack_trace", frames),
		slog.Int("goroutine_id", helpers.GoroutineID()),
	)

	panicErr := recovery.NewPanicError(p, 1)

	return interceptors.NewError(status.New(codes.Internal, "Internal Error"), panicErr)
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

		// If using panics package integration
		panicOpts := panics.NewHandleOpts().SetReallyPanic(false)

		// Create a panicHandler that converts panic to error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContextFromMethod(ctx, method)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = panicHandler(ctx, r, i.opts.logger, meta.FullyMethodName)
			}
		}

		defer panics.HandleWithOpts(ctx, panicOpts, grpcHandler)

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

		// If using panics package integration
		panicOpts := panics.NewHandleOpts().SetReallyPanic(false)

		// Create a panicHandler that converts panic to error
		grpcHandler := func(ctx context.Context, r any) {
			_, meta := metadata.EnsureInContextFromMethod(ctx, method)
			if i.opts.panicHandler != nil {
				err = i.opts.panicHandler(ctx, r)
			} else {
				err = panicHandler(ctx, r, i.opts.logger, meta.FullyMethodName)
			}
		}

		defer panics.HandleWithOpts(ctx, panicOpts, grpcHandler)

		return streamer(ctx, desc, cc, method, opts...)
	}
}
