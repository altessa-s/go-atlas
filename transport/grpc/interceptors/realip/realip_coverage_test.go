// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// fakeServerStream is a minimal grpc.ServerStream double carrying a
// configurable context, used to observe whether the interceptor rewraps
// the stream.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// unparseableAddr is a net.Addr whose String() cannot be parsed by
// netip.ParseAddrPort, exercising the "failed to parse peer address" branch.
type unparseableAddr struct{}

func (unparseableAddr) Network() string { return "fake" }
func (unparseableAddr) String() string  { return "not-an-address" }

func newExtractor(t *testing.T) *clientip.Extractor {
	t.Helper()
	extractor, err := clientip.NewExtractor()
	require.NoError(t, err)
	return extractor
}

func TestServerUnaryInterceptor_SetsRealIPFromPeer(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	inter := ServerUnaryInterceptor(extractor)

	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.5"), Port: 9000}})

	var gotIP netip.Addr
	_, err := inter(ctx, "req", nil, func(handlerCtx context.Context, _ any) (any, error) {
		gotIP = clientip.FromContext(handlerCtx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.True(t, gotIP.IsValid())
	require.Equal(t, "203.0.113.5", gotIP.String())
}

func TestServerUnaryInterceptor_NoPeerInContext(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	inter := ServerUnaryInterceptor(extractor)

	var gotIP netip.Addr
	resp, err := inter(t.Context(), "req", nil, func(handlerCtx context.Context, _ any) (any, error) {
		gotIP = clientip.FromContext(handlerCtx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.False(t, gotIP.IsValid())
}

func TestServerUnaryInterceptor_UnparseablePeerAddr(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	inter := ServerUnaryInterceptor(extractor)

	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: unparseableAddr{}})

	var gotIP netip.Addr
	_, err := inter(ctx, "req", nil, func(handlerCtx context.Context, _ any) (any, error) {
		gotIP = clientip.FromContext(handlerCtx)
		return "ok", nil
	})

	require.NoError(t, err)
	require.False(t, gotIP.IsValid())
}

func TestServerStreamInterceptor_SetsRealIPFromPeer(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	inter := ServerStreamInterceptor(extractor)

	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 443}})
	stream := &fakeServerStream{ctx: ctx}

	var gotIP netip.Addr
	err := inter(nil, stream, nil, func(_ any, s grpc.ServerStream) error {
		gotIP = clientip.FromContext(s.Context())
		return nil
	})

	require.NoError(t, err)
	require.True(t, gotIP.IsValid())
	require.Equal(t, "198.51.100.7", gotIP.String())
}

func TestServerStreamInterceptor_NoPeerPassesThroughUnwrapped(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	inter := ServerStreamInterceptor(extractor)
	stream := &fakeServerStream{ctx: t.Context()}

	var gotStream grpc.ServerStream
	err := inter(nil, stream, nil, func(_ any, s grpc.ServerStream) error {
		gotStream = s
		return nil
	})

	require.NoError(t, err)
	require.Same(t, stream, gotStream, "stream should not be rewrapped when no real IP was found")
}

func TestServerInterceptor_WithLogger_LogsDebugWhenNoPeer(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	extractor := newExtractor(t)
	inter := ServerUnaryInterceptor(extractor, WithLogger(logger))

	_, err := inter(t.Context(), "req", nil, func(ctx context.Context, _ any) (any, error) { return "ok", nil })

	require.NoError(t, err)
	require.Contains(t, buf.String(), "no peer info in context")
}

func TestName_And_ID(t *testing.T) {
	t.Parallel()

	extractor := newExtractor(t)
	i := ServerInterceptor(extractor)
	require.Equal(t, "realip", i.Name())
	require.Equal(t, "realip", Name())
	require.Equal(t, interceptors.Ref("realip"), ID)
}
