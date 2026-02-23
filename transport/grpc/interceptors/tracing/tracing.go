// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/depgraph"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// interceptorName is the name of the tracing interceptor.
const interceptorName = "tracing"

// Interceptor provides tracing for gRPC server and client calls.
type Interceptor struct {
	interceptors.BaseInterceptor
	tracer     tracing.Tracer
	recorder   tracing.Recorder
	opts       *options
	propagator propagation.TextMapPropagator
}

// ServerInterceptor creates a new server tracing interceptor.
func ServerInterceptor(tracer tracing.Tracer, opts ...Option) *Interceptor {
	if tracer == nil {
		tracer = tracing.Noop()
	}
	cfg := newOptions(opts...)
	return &Interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			cfg.ignoreMethods,
			cfg.ignorePatterns,
			cfg.logger,
		),
		tracer:     tracer,
		recorder:   tracer.Recorder("grpc/server"),
		opts:       cfg,
		propagator: cfg.propagator,
	}
}

// ClientInterceptor creates a new client tracing interceptor.
func ClientInterceptor(tracer tracing.Tracer, opts ...Option) *Interceptor {
	if tracer == nil {
		tracer = tracing.Noop()
	}
	cfg := newOptions(opts...)
	return &Interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			cfg.ignoreMethods,
			cfg.ignorePatterns,
			cfg.logger,
		),
		tracer:     tracer,
		recorder:   tracer.Recorder("grpc/client"),
		opts:       cfg,
		propagator: cfg.propagator,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that adds tracing.
func ServerUnaryInterceptor(tracer tracing.Tracer, opts ...Option) grpc.UnaryServerInterceptor {
	return ServerInterceptor(tracer, opts...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that adds tracing.
func ServerStreamInterceptor(tracer tracing.Tracer, opts ...Option) grpc.StreamServerInterceptor {
	return ServerInterceptor(tracer, opts...).ServerStreamInterceptor()
}

// ClientUnaryInterceptor returns a new unary client interceptor that adds tracing.
func ClientUnaryInterceptor(tracer tracing.Tracer, opts ...Option) grpc.UnaryClientInterceptor {
	return ClientInterceptor(tracer, opts...).ClientUnaryInterceptor()
}

// ClientStreamInterceptor returns a new streaming client interceptor that adds tracing.
func ClientStreamInterceptor(tracer tracing.Tracer, opts ...Option) grpc.StreamClientInterceptor {
	return ClientInterceptor(tracer, opts...).ClientStreamInterceptor()
}

// Dependencies returns interceptors that tracing requires to run before it.
func (i *Interceptor) Dependencies() []string {
	return []string{"metadata"}
}

// ServerUnaryInterceptor returns a gRPC unary server interceptor.
func (i *Interceptor) ServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Check if this method should be ignored
		if i.ShouldIgnore(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract trace context from incoming metadata
		ctx = i.extractTraceContext(ctx)

		// Start server span
		spanName := i.opts.spanNameFunc(info.FullMethod)
		ctx, span := i.recorder.Start(ctx, spanName,
			tracing.WithSpanKind(tracing.SpanKindServer),
			tracing.WithAttributes(spanAttributes(info.FullMethod)...),
		)
		defer span.End()

		// Add peer information if available
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			span.SetAttributes(tracing.String(NetPeerNameKey, p.Addr.String()))
		}

		// Call handler
		resp, err := handler(ctx, req)

		// Record status
		i.recordStatus(span, err)

		return resp, err
	}
}

// ServerStreamInterceptor returns a gRPC stream server interceptor.
func (i *Interceptor) ServerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Check if this method should be ignored
		if i.ShouldIgnore(info.FullMethod) {
			return handler(srv, ss)
		}

		ctx := ss.Context()

		// Extract trace context from incoming metadata
		ctx = i.extractTraceContext(ctx)

		// Start server span
		spanName := i.opts.spanNameFunc(info.FullMethod)
		ctx, span := i.recorder.Start(ctx, spanName,
			tracing.WithSpanKind(tracing.SpanKindServer),
			tracing.WithAttributes(spanAttributes(info.FullMethod)...),
		)
		defer span.End()

		// Add peer information if available
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			span.SetAttributes(tracing.String(NetPeerNameKey, p.Addr.String()))
		}

		// Call handler with wrapped stream
		err := handler(srv, interceptors.NewServerWrappedStream(ctx, ss, nil))

		// Record status
		i.recordStatus(span, err)

		return err
	}
}

// ClientUnaryInterceptor returns a gRPC unary client interceptor.
func (i *Interceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Check if this method should be ignored
		if i.ShouldIgnore(method) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		// Start client span
		spanName := i.opts.spanNameFunc(method)
		ctx, span := i.recorder.Start(ctx, spanName,
			tracing.WithSpanKind(tracing.SpanKindClient),
			tracing.WithAttributes(spanAttributes(method)...),
		)
		defer span.End()

		// Inject trace context into outgoing metadata
		ctx = i.injectTraceContext(ctx)

		// Add target information
		if cc != nil && cc.Target() != "" {
			span.SetAttributes(tracing.String(NetPeerNameKey, cc.Target()))
		}

		// Call invoker
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Record status
		i.recordStatus(span, err)

		return err
	}
}

// ClientStreamInterceptor returns a gRPC stream client interceptor.
func (i *Interceptor) ClientStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Check if this method should be ignored
		if i.ShouldIgnore(method) {
			return streamer(ctx, desc, cc, method, opts...)
		}

		// Start client span
		spanName := i.opts.spanNameFunc(method)
		ctx, span := i.recorder.Start(ctx, spanName,
			tracing.WithSpanKind(tracing.SpanKindClient),
			tracing.WithAttributes(spanAttributes(method)...),
		)

		// Inject trace context into outgoing metadata
		ctx = i.injectTraceContext(ctx)

		// Add target information
		if cc != nil && cc.Target() != "" {
			span.SetAttributes(tracing.String(NetPeerNameKey, cc.Target()))
		}

		// Call streamer
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			i.recordStatus(span, err)
			span.End()
			return nil, err
		}

		// Wrap stream to end span when stream closes
		return &tracingClientStream{
			ClientStream: stream,
			span:         span,
		}, nil
	}
}

// extractTraceContext extracts trace context from incoming gRPC metadata.
func (i *Interceptor) extractTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	return i.propagator.Extract(ctx, metadataCarrier(md))
}

// injectTraceContext injects trace context into outgoing gRPC metadata.
func (i *Interceptor) injectTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}

	i.propagator.Inject(ctx, metadataCarrier(md))
	return metadata.NewOutgoingContext(ctx, md)
}

// recordStatus records the gRPC status on the span.
func (i *Interceptor) recordStatus(span tracing.Span, err error) {
	if err == nil {
		span.SetAttributes(tracing.Int(RPCGRPCStatusCodeKey, 0)) // OK
		span.SetStatus(tracing.StatusOK, "")
		return
	}

	st, _ := status.FromError(err)
	span.SetAttributes(tracing.Int(RPCGRPCStatusCodeKey, int(st.Code())))
	span.RecordError(err)
	span.SetStatus(tracing.StatusError, st.Message())
}

// tracingClientStream wraps grpc.ClientStream to end the span on stream close.
type tracingClientStream struct {
	grpc.ClientStream
	span tracing.Span
}

// RecvMsg receives a message and ends the span on EOF or error.
func (s *tracingClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		// Record status code on final error (including EOF)
		st, _ := status.FromError(err)
		s.span.SetAttributes(tracing.Int(RPCGRPCStatusCodeKey, int(st.Code())))
		if err.Error() != "EOF" {
			s.span.RecordError(err)
			s.span.SetStatus(tracing.StatusError, st.Message())
		} else {
			s.span.SetStatus(tracing.StatusOK, "")
		}
		s.span.End()
	}
	return err
}

// metadataCarrier adapts gRPC metadata.MD to propagation.TextMapCarrier.
type metadataCarrier metadata.MD

// Get implements propagation.TextMapCarrier.
func (m metadataCarrier) Get(key string) string {
	vals := metadata.MD(m).Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// Set implements propagation.TextMapCarrier.
func (m metadataCarrier) Set(key, value string) {
	metadata.MD(m).Set(key, value)
}

// Keys implements propagation.TextMapCarrier.
func (m metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Ensure interfaces are implemented.
var (
	_ interceptors.ServerInterceptor = (*Interceptor)(nil)
	_ interceptors.ClientInterceptor = (*Interceptor)(nil)
	_ depgraph.DependencyDeclarer    = (*Interceptor)(nil)
	_ propagation.TextMapCarrier     = metadataCarrier{}
)
