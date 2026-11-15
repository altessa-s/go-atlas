// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

func panicUnaryHandler(context.Context, any) (any, error) { panic("boom") }

func okUnaryHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

func panicStreamHandler(any, grpc.ServerStream) error { panic("boom") }

func TestServerUnaryInterceptor_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	inter := ServerUnaryInterceptor(WithLogger(logger))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := inter(t.Context(), "req", info, panicUnaryHandler)

	require.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
	require.Contains(t, buf.String(), "panic recovered")
	require.Contains(t, buf.String(), "/test.Service/Method")
}

func TestServerUnaryInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	inter := ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := inter(t.Context(), "req", info, okUnaryHandler)

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestServerUnaryInterceptor_CustomPanicHandler(t *testing.T) {
	t.Parallel()

	customErr := errors.New("custom handling")
	inter := ServerUnaryInterceptor(WithPanicHandler(func(context.Context, any) error {
		return customErr
	}))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	resp, err := inter(t.Context(), "req", info, panicUnaryHandler)

	require.Nil(t, resp)
	require.ErrorIs(t, err, customErr)
}

func TestServerStreamInterceptor_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	inter := ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	err := inter(nil, &fakeServerStream{ctx: t.Context()}, info, panicStreamHandler)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestServerStreamInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	inter := ServerStreamInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	sentinel := errors.New("handler result")
	called := false
	err := inter(nil, &fakeServerStream{ctx: t.Context()}, info, func(any, grpc.ServerStream) error {
		called = true
		return sentinel
	})

	require.True(t, called)
	require.ErrorIs(t, err, sentinel, "the handler's result passes through unchanged")
}

func TestClientUnaryInterceptor_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	inter := ClientUnaryInterceptor()
	panicInvoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		panic("client boom")
	}

	err := inter(t.Context(), "/test.Service/Method", "req", "reply", nil, panicInvoker)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestClientUnaryInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	inter := ClientUnaryInterceptor()
	sentinel := errors.New("invoker result")
	var gotMethod string
	invoker := func(_ context.Context, method string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		gotMethod = method
		return sentinel
	}

	err := inter(t.Context(), "/test.Service/Method", "req", "reply", nil, invoker)

	require.Equal(t, "/test.Service/Method", gotMethod)
	require.ErrorIs(t, err, sentinel, "the invoker's result passes through unchanged")
}

func TestClientUnaryInterceptor_CustomPanicHandler(t *testing.T) {
	t.Parallel()

	customErr := errors.New("client custom")
	inter := ClientUnaryInterceptor(WithPanicHandler(func(context.Context, any) error {
		return customErr
	}))
	panicInvoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		panic("client boom")
	}

	err := inter(t.Context(), "/test.Service/Method", "req", "reply", nil, panicInvoker)

	require.ErrorIs(t, err, customErr)
}

func TestClientStreamInterceptor_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	inter := ClientStreamInterceptor()
	panicStreamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		panic("stream boom")
	}

	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/test.Service/Stream", panicStreamer)

	require.Nil(t, stream)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestClientStreamInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	inter := ClientStreamInterceptor()
	okStreamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}

	stream, err := inter(t.Context(), &grpc.StreamDesc{}, nil, "/test.Service/Stream", okStreamer)

	require.NoError(t, err)
	require.Nil(t, stream)
}
