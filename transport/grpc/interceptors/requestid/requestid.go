// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/validation"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	stdGrpc "google.golang.org/grpc"
)

const interceptorName = "requestid"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

// ErrInvalidRequestId is returned by [ServerInterceptor] and [ServerUnaryInterceptor]
// when the incoming request carries a request ID that is not a valid UUID v4 and
// the generator is not configured to generate missing IDs. The error is surfaced
// as a gRPC [codes.InvalidArgument] status.
var ErrInvalidRequestId = errors.New("invalid request ID, it must be a UUID v4")

// grpcHeaderGetter adapts gRPC metadata to the HeaderGetter interface.
type grpcHeaderGetter struct {
	md metadata.MD
}

// GetHeader returns the first value for the given metadata key.
func (g *grpcHeaderGetter) GetHeader(name string) string {
	if g.md == nil {
		return ""
	}
	if vals := g.md.Get(name); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// ServerInterceptor returns a new interceptor that sets the request ID in the context.
// If the request ID is already set in the context, it will be used as is, otherwise, a new request ID will be generated.
// If request ID is not a valid UUID v4, it will return an error ErrInvalidRequestId.
func ServerInterceptor(gen *requestid.Generator) interceptors.ServerInterceptor {
	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor(interceptorName, nil),
		gen:             gen,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that sets the request ID in the context.
// If the request ID is already set in the context, it will be used as is, otherwise, a new request ID will be generated.
// If request ID is not a valid UUID v4, it will return an error.
func ServerUnaryInterceptor(gen *requestid.Generator) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(gen).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that sets the real IP address in the context.
func ServerStreamInterceptor(gen *requestid.Generator) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(gen).ServerStreamInterceptor()
}

// ClientInterceptor returns a new interceptor that propagates request ID from context to outgoing metadata.
// If no request ID is found in context and WithGenerateIfMissing is true, a new UUID v4 will be generated.
func ClientInterceptor(gen *requestid.Generator) interceptors.ClientInterceptor {
	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor(interceptorName, nil),
		gen:             gen,
	}
}

// ClientUnaryInterceptor returns a new unary client interceptor that propagates request ID.
func ClientUnaryInterceptor(gen *requestid.Generator) stdGrpc.UnaryClientInterceptor {
	return ClientInterceptor(gen).ClientUnaryInterceptor()
}

// ClientStreamInterceptor returns a new streaming client interceptor that propagates request ID.
func ClientStreamInterceptor(gen *requestid.Generator) stdGrpc.StreamClientInterceptor {
	return ClientInterceptor(gen).ClientStreamInterceptor()
}

var _ interceptors.ServerInterceptor = (*interceptor)(nil)
var _ interceptors.ClientInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	gen *requestid.Generator
}

// Dependencies returns interceptors that requestid requires to run before it.
// Requestid uses metadata for call information extraction.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name()}
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		ctx, _ = sharedmetadata.EnsureInContext(ctx, info.FullMethod, info)

		ctx, err := i.contextWithRequestID(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, ss stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		ctx, _ := sharedmetadata.EnsureInContext(ss.Context(), info.FullMethod, info)

		ctx, err := i.contextWithRequestID(ctx)
		if err != nil {
			return err
		}
		return handler(srv, interceptors.NewServerWrappedStream(ctx, ss, nil))
	}
}

// NewContext returns a new context with the given request ID.
// This is useful for injecting a request ID into the context for client calls.
func NewContext(ctx context.Context, id string) context.Context {
	return requestid.NewContext(ctx, id)
}

// FromContext returns the request ID from the context.
func FromContext(ctx context.Context) string {
	return requestid.FromContext(ctx)
}

// ClientUnaryInterceptor returns a new unary client interceptor that propagates request ID.
func (i *interceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *stdGrpc.ClientConn, invoker stdGrpc.UnaryInvoker, opts ...stdGrpc.CallOption) error {
		ctx = i.clientAttachRequestID(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientStreamInterceptor returns a new streaming client interceptor that propagates request ID.
func (i *interceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stdGrpc.ClientStream, error) {
		ctx = i.clientAttachRequestID(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// clientAttachRequestID extracts request ID from context and attaches it to outgoing metadata.
// If no request ID is found and generateIfMissing is true, generates a new UUID v4.
func (i *interceptor) clientAttachRequestID(ctx context.Context) context.Context {
	// First check if request ID exists in context
	reqID := requestid.FromContext(ctx)

	if reqID == "" {
		if i.gen.GenerateIfMissing() {
			reqID = i.gen.Extract(nil)
			ctx = requestid.NewContext(ctx, reqID)
		}
		return ctx // No ID and generation disabled, return original context
	}

	// If we have a request ID, attach it to outgoing metadata. The metadata
	// map must stay live for the whole RPC, so it is built fresh here — never
	// pooled and recycled.
	var md metadata.MD
	if existingMD, ok := metadata.FromOutgoingContext(ctx); ok {
		md = existingMD.Copy()
		md.Set(i.gen.HeaderName(), reqID)
	} else {
		md = metadata.Pairs(i.gen.HeaderName(), reqID)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func (i *interceptor) contextWithRequestID(ctx context.Context) (context.Context, error) {
	var headers *grpcHeaderGetter
	var raw string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		headers = &grpcHeaderGetter{md: md}
		raw = headers.GetHeader(i.gen.HeaderName())
	}

	if !i.gen.GenerateIfMissing() {
		if raw == "" || !validation.IsValidUUIDv4(raw) {
			return ctx, status.Error(codes.InvalidArgument, ErrInvalidRequestId.Error())
		}
	}

	reqID := i.gen.Extract(headers)
	if reqID == "" {
		return ctx, nil // No valid request ID, return original context
	}

	_ = stdGrpc.SetHeader(ctx, metadata.Pairs(i.gen.HeaderName(), reqID)) //nolint:errcheck
	return requestid.NewContext(ctx, reqID), nil
}
