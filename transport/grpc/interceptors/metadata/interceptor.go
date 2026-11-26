// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"context"
	"sync"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

const interceptorName = "metadata"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// namer satisfies interceptors.Interceptor without importing the parent package.
type namer string

func (n namer) Name() string { return string(n) }

// ID is a lightweight interceptor reference for this package,
// suitable for passing to exclusion lists.
var ID namer = interceptorName

// metadataInterceptor handles initialization of CallMetadata in the context.
type metadataInterceptor struct{}

func (i metadataInterceptor) Name() string { return interceptorName }

// Dependencies returns nil as metadata has no dependencies.
// Metadata is the root interceptor that all others depend on.
func (i metadataInterceptor) Dependencies() []string { return nil }

func (i metadataInterceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is already passed in 'c' and injected into context by the caller
	// of DrivenInterceptor if we use our standard wrappers.
	return driver.NoopDriver(), ctx
}

// Interceptor returns a [driver.DrivenInterceptor] that ensures [CallMetadata]
// is present in the context for all downstream interceptors. The [Chain]
// automatically prepends this interceptor if none is registered with the
// name "metadata".
func Interceptor() driver.DrivenInterceptor {
	return metadataInterceptor{}
}

// UnknownMethod is the telemetry name for requests to an unregistered RPC.
const UnknownMethod = "/_unknown/Unknown"

type registeredMethodsKey struct{}

type registeredMethods map[string]struct{}

// ServerMethodOptions installs the registered method set for metadata cache
// admission and unknown stream classification. Pass a callback to the serving
// server's GetServiceInfo. It is called once, on the first RPC after startup.
// Place these options before other grpc.Chain*Interceptor options.
func ServerMethodOptions(serviceInfo func() map[string]grpc.ServiceInfo) []grpc.ServerOption {
	methods := sync.OnceValue(func() registeredMethods {
		set := make(registeredMethods)
		for service, info := range serviceInfo() {
			for _, method := range info.Methods {
				set["/"+service+"/"+method.Name] = struct{}{}
			}
		}
		return set
	})
	withMethods := func(ctx context.Context) context.Context {
		return context.WithValue(ctx, registeredMethodsKey{}, methods())
	}
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			return next(withMethods(ctx), req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, next grpc.StreamHandler) error {
			return next(srv, &registeredMethodStream{ServerStream: stream, ctx: withMethods(stream.Context())})
		}),
	}
}

type registeredMethodStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *registeredMethodStream) Context() context.Context { return s.ctx }

// ServerStreamMethod returns a bounded method name for calls dispatched
// through grpc.UnknownServiceHandler. Without a registered method set, it
// falls back to grpc-go's unknown-service dispatch convention. The transport
// retains the original path for the unknown service handler.
func ServerStreamMethod(srv any, ctx context.Context, info *grpc.StreamServerInfo) string {
	if methods, ok := ctx.Value(registeredMethodsKey{}).(registeredMethods); ok {
		if _, registered := methods[info.FullMethod]; registered {
			return info.FullMethod
		}
		return UnknownMethod
	}
	if srv == nil && info.IsClientStream && info.IsServerStream && grpc.ServerTransportStreamFromContext(ctx) != nil {
		return UnknownMethod
	}
	return info.FullMethod
}

// EnsureServerStreamInContext initializes metadata with the bounded server
// method name before the method reaches the cache.
func EnsureServerStreamInContext(srv any, ctx context.Context, info *grpc.StreamServerInfo) (context.Context, *CallMetadata) {
	return EnsureInContext(ctx, ServerStreamMethod(srv, ctx, info), info)
}
