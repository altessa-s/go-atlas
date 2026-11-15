// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	obstracing "github.com/altessa-s/go-atlas/observability/tracing"
)

// newBufferedTracer builds a real [obstracing.Tracer] that exports spans
// synchronously (console adapter, AlwaysOn sampler) into buf, so tests
// can assert on observable span output instead of internal state.
func newBufferedTracer(buf *bytes.Buffer) obstracing.Tracer {
	return obstracing.New(obstracing.WithAdapter(console.New(console.WithWriter(buf))))
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// fakeClientStream is a minimal grpc.ClientStream double whose RecvMsg
// behavior is configurable, used to exercise tracingClientStream.RecvMsg.
type fakeClientStream struct {
	grpc.ClientStream
	recvErr error
}

func (f *fakeClientStream) RecvMsg(any) error { return f.recvErr }

// fakePropagator is a no-op [propagation.TextMapPropagator] used to
// exercise the extractTraceContext fallback path taken for any
// propagator other than [*propagation.TraceContext].
type fakePropagator struct {
	injected  map[string]string
	extracted string
}

func (p *fakePropagator) Inject(_ context.Context, carrier propagation.TextMapCarrier) {
	if p.injected == nil {
		p.injected = map[string]string{}
	}
	carrier.Set("x-fake-trace", "1")
	p.injected["x-fake-trace"] = "1"
}

func (p *fakePropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	p.extracted = carrier.Get("x-fake-trace")
	return ctx
}

func (p *fakePropagator) Fields() []string { return []string{"x-fake-trace"} }

func TestServerUnaryInterceptor_SuccessRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerUnaryInterceptor(tracer)
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}

	resp, err := inter(t.Context(), "req", info, func(_ context.Context, req any) (any, error) {
		return "ok", nil
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Contains(t, buf.String(), "/pkg.Svc/Method")
	require.Contains(t, buf.String(), " OK\n")
}

func TestServerUnaryInterceptor_ErrorRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerUnaryInterceptor(tracer)
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handlerErr := status.Error(codes.NotFound, "missing")

	resp, err := inter(t.Context(), "req", info, func(context.Context, any) (any, error) {
		return nil, handlerErr
	})

	require.Nil(t, resp)
	require.ErrorIs(t, err, handlerErr)
	require.Contains(t, buf.String(), " Error\n")
}

func TestServerUnaryInterceptor_RecordsPeerAttribute(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerUnaryInterceptor(tracer)
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}

	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}})

	_, err := inter(ctx, "req", info, func(_ context.Context, _ any) (any, error) { return "ok", nil })

	require.NoError(t, err)
	require.Contains(t, buf.String(), "net.peer.name=127.0.0.1:1234")
}

func TestServerStreamInterceptor_SuccessRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerStreamInterceptor(tracer)
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}

	err := inter(nil, stream, info, func(_ any, _ grpc.ServerStream) error { return nil })

	require.NoError(t, err)
	require.Contains(t, buf.String(), "/pkg.Svc/Stream")
	require.Contains(t, buf.String(), " OK\n")
}

func TestServerStreamInterceptor_ErrorRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerStreamInterceptor(tracer)
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	stream := &fakeServerStream{ctx: t.Context()}
	handlerErr := status.Error(codes.Unavailable, "down")

	err := inter(nil, stream, info, func(any, grpc.ServerStream) error { return handlerErr })

	require.ErrorIs(t, err, handlerErr)
	require.Contains(t, buf.String(), " Error\n")
}

func TestClientUnaryInterceptor_SuccessRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ClientUnaryInterceptor(tracer)

	var invokerCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCtx = ctx
		return nil
	}

	err := inter(t.Context(), "/pkg.Svc/Method", "req", "reply", nil, invoker)

	require.NoError(t, err)
	require.Contains(t, buf.String(), " OK\n")

	// injectTraceContext must have stashed the propagated headers in the
	// outgoing metadata the invoker receives.
	_, ok := metadata.FromOutgoingContext(invokerCtx)
	require.True(t, ok)
}

func TestClientUnaryInterceptor_ErrorRecordsSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ClientUnaryInterceptor(tracer)
	invokerErr := status.Error(codes.PermissionDenied, "denied")

	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		return invokerErr
	}

	err := inter(t.Context(), "/pkg.Svc/Method", "req", "reply", nil, invoker)

	require.ErrorIs(t, err, invokerErr)
	require.Contains(t, buf.String(), " Error\n")
}

func TestClientStreamInterceptor_StreamerErrorRecordsSpanImmediately(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ClientStreamInterceptor(tracer)
	streamerErr := status.Error(codes.Unavailable, "down")

	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, streamerErr
	}

	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)

	require.Nil(t, stream)
	require.ErrorIs(t, err, streamerErr)
	require.Contains(t, buf.String(), " Error\n")
}

func TestClientStreamInterceptor_WrapsStreamAndEndsSpanOnError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ClientStreamInterceptor(tracer)
	recvErr := status.Error(codes.Internal, "boom")

	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return &fakeClientStream{recvErr: recvErr}, nil
	}

	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	require.Empty(t, buf.String()) // span is only ended by RecvMsg, not by a successful streamer call

	gotErr := stream.RecvMsg(new(string))

	require.ErrorIs(t, gotErr, recvErr)
	require.Contains(t, buf.String(), " Error\n")
}

func TestClientStreamInterceptor_WrapsStreamAndEndsSpanOnEOF(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ClientStreamInterceptor(tracer)

	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return &fakeClientStream{recvErr: io.EOF}, nil
	}

	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)

	gotErr := stream.RecvMsg(new(string))

	require.ErrorIs(t, gotErr, io.EOF)
	require.Contains(t, buf.String(), " OK\n")
}

func TestExtractTraceContext_CustomPropagator(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	fp := &fakePropagator{}
	inter := ServerInterceptor(tracer, WithPropagator(fp))
	unary := inter.ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}

	md := metadata.New(map[string]string{"x-fake-trace": "1"})
	ctx := metadata.NewIncomingContext(t.Context(), md)

	_, err := unary(ctx, "req", info, func(_ context.Context, _ any) (any, error) { return "ok", nil })

	require.NoError(t, err)
	require.Equal(t, "1", fp.extracted, "the custom propagator extracts from the incoming metadata")
}

func TestWithSpanNameFunc(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tracer := newBufferedTracer(&buf)
	inter := ServerUnaryInterceptor(tracer, WithSpanNameFunc(func(string) string { return "custom-span" }))
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}

	_, err := inter(t.Context(), "req", info, func(context.Context, any) (any, error) { return "ok", nil })

	require.NoError(t, err)
	require.Contains(t, buf.String(), "custom-span")
}

func TestClientInterceptor_Dependencies(t *testing.T) {
	t.Parallel()

	i := ClientInterceptor(nil)
	deps := i.Dependencies()
	require.Equal(t, []string{"metadata"}, deps)
}
