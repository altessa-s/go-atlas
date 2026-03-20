// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

// Ensure interceptor implements the ServerInterceptor interface.
var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	registry *ipacl.Registry
	opts     *options
}

// Dependencies returns optional interceptors that should run before ipacl.
func (i *interceptor) Dependencies() []string {
	return nil
}

// RequiredDependencies returns interceptors that ipacl requires to function.
func (i *interceptor) RequiredDependencies() []string {
	return []string{"realip"}
}

// ServerInterceptor returns a new interceptor that enforces IP-based access control.
func ServerInterceptor(registry *ipacl.Registry, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"ipacl",
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		registry: registry,
		opts:     opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that enforces IP-based access control.
func ServerUnaryInterceptor(registry *ipacl.Registry, opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(registry, opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that enforces IP-based access control.
func ServerStreamInterceptor(registry *ipacl.Registry, opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(registry, opt...).ServerStreamInterceptor()
}

// ServerUnaryInterceptor returns a unary server interceptor for IP access control.
func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		if err := i.checkAccess(ctx, i.InternMethod(info.FullMethod)); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns a streaming server interceptor for IP access control.
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

	if !i.registry.Evaluate(ip, method) {
		i.LogDebug(ctx, "access denied", method)
		return status.Error(codes.PermissionDenied, "Access Denied")
	}

	i.LogDebug(ctx, "access granted", method)
	return nil
}
