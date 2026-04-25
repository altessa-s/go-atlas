// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

const interceptorName = "geoacl"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

// Ensure interceptor implements the ServerInterceptor interface.
var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	resolver geoacl.GeoResolver
	registry *geoacl.Registry
	opts     *options
}

// Dependencies returns optional interceptors that should run before geoacl.
func (i *interceptor) Dependencies() []string {
	return nil
}

// RequiredDependencies returns interceptors that geoacl requires to function.
func (i *interceptor) RequiredDependencies() []string {
	return []string{realip.Name()}
}

// ServerInterceptor returns a new interceptor that enforces geographic access control.
func ServerInterceptor(resolver geoacl.GeoResolver, registry *geoacl.Registry, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		resolver: resolver,
		registry: registry,
		opts:     opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that enforces geographic access control.
func ServerUnaryInterceptor(resolver geoacl.GeoResolver, registry *geoacl.Registry, opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(resolver, registry, opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that enforces geographic access control.
func ServerStreamInterceptor(resolver geoacl.GeoResolver, registry *geoacl.Registry, opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(resolver, registry, opt...).ServerStreamInterceptor()
}

// ServerUnaryInterceptor returns a unary server interceptor for geographic access control.
func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		if err := i.checkAccess(ctx, i.InternMethod(info.FullMethod)); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns a streaming server interceptor for geographic access control.
func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		if err := i.checkAccess(stream.Context(), i.InternMethod(info.FullMethod)); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func (i *interceptor) checkAccess(ctx context.Context, method string) error {
	if i.ShouldIgnore(method) {
		i.LogIgnored(ctx, method)
		return nil
	}

	ip := clientip.FromContext(ctx)
	if !ip.IsValid() {
		i.LogDebug(ctx, "no valid client IP", method)
		switch i.opts.fallbackBehavior {
		case fallback.Allow:
			return nil
		default:
			return status.Error(codes.PermissionDenied, "Access Denied")
		}
	}

	allowed, err := i.registry.Evaluate(ctx, i.resolver, ip, method)
	if err != nil {
		i.LogDebug(ctx, "geo resolver error", method, slog.String("error", err.Error()))
		switch i.opts.fallbackBehavior {
		case fallback.Allow:
			return nil
		case fallback.Error:
			return status.Errorf(codes.Internal, "geo resolver error: %v", err)
		default:
			return status.Error(codes.PermissionDenied, "Access Denied")
		}
	}

	if !allowed {
		i.LogDebug(ctx, "access denied", method)
		return status.Error(codes.PermissionDenied, "Access Denied")
	}

	i.LogDebug(ctx, "access granted", method)
	return nil
}
