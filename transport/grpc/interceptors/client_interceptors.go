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

// DrivenInterceptorFunc adapts functions to the DrivenInterceptor interface.
type DrivenInterceptorFunc func(context.Context) (driver.Driver, context.Context)

// DrivenInterceptor implements the DrivenInterceptor interface.
func (f DrivenInterceptorFunc) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	return f(ctx)
}

// ClientInterceptor provides client-side interception for both unary and streaming RPCs.
// Implementations must also satisfy [Interceptor] (return a unique name for
// dependency ordering). Register client interceptors with [Chain.ClientOptions].
//
// For interceptors that follow the driven pattern, use [ClientDrivenInterceptor]
// to adapt a [driver.DrivenInterceptor] to this interface.
type ClientInterceptor interface {
	Interceptor
	ClientUnaryInterceptor() grpc.UnaryClientInterceptor
	ClientStreamInterceptor() grpc.StreamClientInterceptor
}

// DrivenClientInterceptor adapts a DrivenInterceptor to the ClientInterceptor interface.
type DrivenClientInterceptor struct{ i driver.DrivenInterceptor }

// ClientDrivenInterceptor adapts DrivenInterceptor to ClientInterceptor.
//
// Example:
//
//	ci := interceptors.ClientDrivenInterceptor(driven)
//	conn, err := grpc.Dial(target,
//	    grpc.WithUnaryInterceptor(ci.ClientUnaryInterceptor()),
//	    grpc.WithStreamInterceptor(ci.ClientStreamInterceptor()),
//	)
func ClientDrivenInterceptor(i driver.DrivenInterceptor) ClientInterceptor {
	return &DrivenClientInterceptor{i: i}
}

// Name returns the name of the interceptor.
func (i *DrivenClientInterceptor) Name() string {
	if named, ok := i.i.(Interceptor); ok {
		return named.Name()
	}
	return "driven"
}

// Interceptor returns the underlying DrivenInterceptor.
func (i *DrivenClientInterceptor) Interceptor() any {
	return i.i
}

// ClientUnaryInterceptor returns a unary client interceptor.
func (i *DrivenClientInterceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, _ = metadata.EnsureInContextFromMethod(ctx, method)
		h, newCtx := i.i.DrivenInterceptor(ctx)

		_, err := h.PreCall(newCtx, req)
		if err != nil {
			return err
		}

		err = invoker(newCtx, method, req, reply, cc, opts...)

		return h.PostCall(newCtx, reply, err)
	}
}

// ClientStreamInterceptor returns a streaming client interceptor.
func (i *DrivenClientInterceptor) ClientStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, _ = metadata.EnsureInContextFromMethod(ctx, method)
		h, newCtx := i.i.DrivenInterceptor(ctx)
		if _, err := h.PreCall(newCtx, nil); err != nil {
			return nil, err
		}

		stream, err := streamer(newCtx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}

		return NewClientStreamWrapper(newCtx, stream, h), nil
	}
}

// ClientConditionalInterceptor applies interceptor based on condition.
// Returns no-op interceptor if condition is false.
//
// Example:
//
//	retry := interceptors.ClientConditionalInterceptor(
//	    config.RetryEnabled,
//	    retry.ClientInterceptor(retryPolicy),
//	)
func ClientConditionalInterceptor(cond bool, i ClientInterceptor) ClientInterceptor {
	if cond {
		return i
	}
	return &NoOpClientInterceptor{}
}

// ClientConditionalInterceptorFunc creates interceptor if condition is true.
// Useful for expensive interceptor initialization.
func ClientConditionalInterceptorFunc(cond bool, fn func() ClientInterceptor) ClientInterceptor {
	if cond {
		return fn()
	}
	return &NoOpClientInterceptor{}
}

// ClientMatchInterceptor applies interceptor based on dynamic matcher.
//
// Example:
//
//	matcher := interceptors.MatchFunc(func() bool {
//	    return healthCheck.IsHealthy("service")
//	})
//	fastPath := interceptors.ClientMatchInterceptor(
//	    matcher,
//	    fastpath.ClientInterceptor(),
//	)
func ClientMatchInterceptor(m Matcher, i ClientInterceptor) ClientInterceptor {
	if m.Match() {
		return i
	}
	return &NoOpClientInterceptor{}
}

// ClientMatchInterceptorFunc creates interceptor based on dynamic matcher.
func ClientMatchInterceptorFunc(m MatchFunc, fn func() ClientInterceptor) ClientInterceptor {
	if m.Match() {
		return fn()
	}
	return &NoOpClientInterceptor{}
}

// OrderClientInterceptors sorts interceptors by their declared dependencies using topological sort.
// Duplicates (same name) are removed, keeping the first occurrence.
// Returns an error if a circular dependency is detected.
func OrderClientInterceptors(interceptors ...ClientInterceptor) ([]ClientInterceptor, error) {
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
	result := make([]ClientInterceptor, 0, len(sorted))
	for _, item := range sorted {
		if ic, ok := item.(ClientInterceptor); ok {
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
