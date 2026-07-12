// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

var (
	benchResp any
	benchErr  error
)

// benchServerUnary invokes the unary interceptor with a preconfigured request
// context, measuring the per-request IP extraction and context injection cost.
func benchServerUnary(b *testing.B, ctx context.Context) {
	b.Helper()

	extractor, err := clientip.NewExtractor()
	require.NoError(b, err)

	unary := ServerUnaryInterceptor(extractor)
	info := &grpc.UnaryServerInfo{FullMethod: "/bench.EchoService/Echo"}
	handler := func(ctx context.Context, req any) (any, error) { return req, nil }

	b.ReportAllocs()
	for b.Loop() {
		benchResp, benchErr = unary(ctx, "req", info, handler)
	}
}

// BenchmarkServerUnaryInterceptor_PublicPeer measures the short-circuit path:
// the peer address is public and untrusted, so it is used directly without
// consulting proxy headers.
func BenchmarkServerUnaryInterceptor_PublicPeer(b *testing.B) {
	ctx := peer.NewContext(b.Context(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 51234},
	})
	benchServerUnary(b, ctx)
}

// BenchmarkServerUnaryInterceptor_ForwardedHeader measures the full header
// extraction path: a private peer address forces a walk of the proxy headers,
// resolving the client IP from X-Forwarded-For in incoming metadata.
func BenchmarkServerUnaryInterceptor_ForwardedHeader(b *testing.B) {
	ctx := peer.NewContext(b.Context(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 51234},
	})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-forwarded-for", "203.0.113.7"))
	benchServerUnary(b, ctx)
}
