// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"

	grpcmetadata "google.golang.org/grpc/metadata"
)

// mockClientStream implements grpc.ClientStream for testing.
type mockClientStream struct {
	grpc.ClientStream
	ctx     context.Context
	sendErr error
	recvErr error
}

func (m *mockClientStream) Context() context.Context         { return m.ctx }
func (m *mockClientStream) SendMsg(msg any) error            { return m.sendErr }
func (m *mockClientStream) RecvMsg(msg any) error            { return m.recvErr }
func (m *mockClientStream) CloseSend() error                 { return nil }
func (m *mockClientStream) Header() (grpcmetadata.MD, error) { return nil, nil }
func (m *mockClientStream) Trailer() grpcmetadata.MD         { return nil }

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx     context.Context
	sendErr error
	recvErr error
}

func (m *mockServerStream) Context() context.Context         { return m.ctx }
func (m *mockServerStream) SendMsg(msg any) error            { return m.sendErr }
func (m *mockServerStream) RecvMsg(msg any) error            { return m.recvErr }
func (m *mockServerStream) SetHeader(grpcmetadata.MD) error  { return nil }
func (m *mockServerStream) SendHeader(grpcmetadata.MD) error { return nil }
func (m *mockServerStream) SetTrailer(grpcmetadata.MD)       {}

func TestClientStreamWrapper_Context(t *testing.T) {
	ctx := context.WithValue(t.Context(), struct{}{}, "val")
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(ctx, cs, NoopDriver())
	require.Equal(t, ctx, w.Context())
}

func TestClientStreamWrapper_SendMsg(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.SendMsg("msg")
	require.NoError(t, err)
}

func TestClientStreamWrapper_RecvMsg(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.RecvMsg(nil)
	require.NoError(t, err)
}

func TestClientStreamWrapper_CloseSend(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	err := w.CloseSend()
	require.NoError(t, err)
}

func TestClientStreamWrapper_Reuse(t *testing.T) {
	cs := &mockClientStream{ctx: t.Context()}
	w1 := NewClientStreamWrapper(t.Context(), cs, NoopDriver())
	ctx2 := context.WithValue(t.Context(), struct{}{}, "v2")
	w2 := NewClientStreamWrapper(ctx2, w1, NoopDriver())
	require.Equal(t, w1, w2)
	require.Equal(t, ctx2, w2.Context())
}

func TestServerStreamWrapper_Context(t *testing.T) {
	ctx := context.WithValue(t.Context(), struct{}{}, "val")
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(ctx, ss, NoopDriver())
	require.Equal(t, ctx, w.Context())
}

func TestServerStreamWrapper_SendMsg(t *testing.T) {
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(t.Context(), ss, NoopDriver())
	err := w.SendMsg("msg")
	require.NoError(t, err)
}

func TestServerStreamWrapper_RecvMsg(t *testing.T) {
	ss := &mockServerStream{ctx: t.Context()}
	w := NewServerWrappedStream(t.Context(), ss, NoopDriver())
	err := w.RecvMsg(nil)
	require.NoError(t, err)
}

func TestServerStreamWrapper_Reuse(t *testing.T) {
	ss := &mockServerStream{ctx: t.Context()}
	w1 := NewServerWrappedStream(t.Context(), ss, NoopDriver())
	ctx2 := context.WithValue(t.Context(), struct{}{}, "v2")
	w2 := NewServerWrappedStream(ctx2, w1, NoopDriver())
	require.Equal(t, w1, w2)
}
