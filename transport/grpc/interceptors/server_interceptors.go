// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc"
)

// ServerInterceptor provides server-side interception for both unary and streaming RPCs.
// Implementations must also satisfy [Interceptor] (return a unique name for
// dependency ordering). Register server interceptors with [Chain] or directly
// with [grpc.Server] via [Server.RegisterInterceptors].
//
// For interceptors that follow the driven pattern, use [ServerDrivenInterceptor]
// to adapt a [driver.DrivenInterceptor] to this interface.
type ServerInterceptor interface {
	Interceptor
	ServerUnaryInterceptor() grpc.UnaryServerInterceptor
	ServerStreamInterceptor() grpc.StreamServerInterceptor
}

// DrivenServerInterceptor adapts a DrivenInterceptor to the ServerInterceptor interface.
type DrivenServerInterceptor struct{ i driver.DrivenInterceptor }

// ServerDrivenInterceptor adapts DrivenInterceptor to ServerInterceptor.
//
// Example:
//
//	si := interceptors.ServerDrivenInterceptor(driven)
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(si.ServerUnaryInterceptor()),
//	    grpc.StreamInterceptor(si.ServerStreamInterceptor()),
//	)
func ServerDrivenInterceptor(i driver.DrivenInterceptor) ServerInterceptor {
	return &DrivenServerInterceptor{i: i}
}

// Name returns the name of the interceptor.
func (i *DrivenServerInterceptor) Name() string {
	if named, ok := i.i.(Interceptor); ok {
		return named.Name()
	}
	return "driven"
}

// ServerUnaryInterceptor returns a unary server interceptor.
func (i *DrivenServerInterceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, _ = metadata.EnsureInContext(ctx, info.FullMethod, info)
		d, newCtx := i.i.DrivenInterceptor(ctx)

		resp, err := d.PreCall(newCtx, req)
		if err != nil {
			return nil, err
		} else if resp != nil {
			// If PreCall returned a response, we skip the handler.
			return resp, nil
		}

		resp, err = handler(newCtx, req)

		postErr := d.PostCall(newCtx, resp, err)
		if postErr != nil {
			return nil, postErr
		}

		return resp, err
	}
}

// Interceptor returns the underlying DrivenInterceptor.
func (i *DrivenServerInterceptor) Interceptor() any {
	return i.i
}

// ServerStreamInterceptor returns a streaming server interceptor.
func (i *DrivenServerInterceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, _ := metadata.EnsureInContext(stream.Context(), info.FullMethod, info)

		d, newCtx := i.i.DrivenInterceptor(ctx)

		// For streaming RPCs, we are ignoring the response from PreCall.
		if _, err := d.PreCall(newCtx, nil); err != nil {
			return err
		}

		err := handler(srv, NewServerWrappedStream(newCtx, stream, d))

		return d.PostCall(newCtx, nil, err)
	}
}

// ServerConditionalInterceptor applies interceptor based on condition.
// Returns no-op interceptor if condition is false.
//
// Example:
//
//	auth := interceptors.ServerConditionalInterceptor(
//	    config.AuthEnabled,
//	    auth.ServerInterceptor(authFunc),
//	)
func ServerConditionalInterceptor(cond bool, i ServerInterceptor) ServerInterceptor {
	if cond {
		return i
	}
	return &NoOpInterceptor{}
}

// ServerConditionalInterceptorFunc creates interceptor if condition is true.
// Useful for expensive interceptor initialization.
func ServerConditionalInterceptorFunc(cond bool, fn func() ServerInterceptor) ServerInterceptor {
	if cond {
		return fn()
	}
	return &NoOpInterceptor{}
}

// ServerMatchInterceptor applies interceptor based on dynamic matcher.
//
// Example:
//
//	matcher := interceptors.MatchFunc(func() bool {
//	    return featureFlags.IsEnabled("auth-v2")
//	})
//	auth := interceptors.ServerMatchInterceptor(
//	    matcher,
//	    auth.ServerInterceptor(authV2),
//	)
func ServerMatchInterceptor(m Matcher, i ServerInterceptor) ServerInterceptor {
	if m.Match() {
		return i
	}
	return &NoOpInterceptor{}
}

// ServerMatchInterceptorFunc creates interceptor based on dynamic matcher.
func ServerMatchInterceptorFunc(m MatchFunc, fn func() ServerInterceptor) ServerInterceptor {
	if m.Match() {
		return fn()
	}
	return &NoOpInterceptor{}
}

// OrderServerInterceptors sorts interceptors by their declared dependencies using topological sort.
// Duplicates (same name) are removed, keeping the first occurrence.
// Returns an error if a circular dependency is detected.
func OrderServerInterceptors(interceptors ...ServerInterceptor) ([]ServerInterceptor, error) {
	// Convert to []any for dependency ordering
	items := make([]any, len(interceptors))
	for i, ic := range interceptors {
		items[i] = ic
	}

	// Order by dependencies
	sorted, err := orderByDependencies(items, nil)
	if err != nil {
		return nil, err
	}

	// Convert back and remove duplicates
	seen := make(map[string]struct{})
	result := make([]ServerInterceptor, 0, len(sorted))
	for _, item := range sorted {
		if ic, ok := item.(ServerInterceptor); ok {
			name := ""
			if named, ok := item.(Interceptor); ok {
				name = named.Name()
			}
			if name != "" {
				if _, exists := seen[name]; exists {
					continue
				}
				seen[name] = struct{}{}
			}
			result = append(result, ic)
		}
	}
	return result, nil
}
