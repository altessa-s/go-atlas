// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"context"
	"log/slog"
	"net/netip"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/headers"

	"google.golang.org/grpc/peer"

	stdGrpc "google.golang.org/grpc"
)

const extractOperation = "extract"

// ServerInterceptor returns a new interceptor that sets the real IP address in the context.
// The extractor parameter configures how IP addresses are extracted from headers
// and which proxies are trusted.
func ServerInterceptor(extractor *clientip.Extractor, opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)
	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("realip", opts.logger),
		extractor:       extractor,
		opts:            opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that sets the real IP address in the context.
func ServerUnaryInterceptor(extractor *clientip.Extractor, opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(extractor, opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that sets the real IP address in the context.
func ServerStreamInterceptor(extractor *clientip.Extractor, opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(extractor, opt...).ServerStreamInterceptor()
}

var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	extractor *clientip.Extractor
	opts      *options
}

func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		if ip := i.getIP(ctx); ip != nil {
			ctx = clientip.NewContext(ctx, *ip)
		}
		return handler(ctx, req)
	}
}

func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, _ *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		if ip := i.getIP(stream.Context()); ip != nil {
			stream = interceptors.NewServerWrappedStream(clientip.NewContext(stream.Context(), *ip), stream, nil)
		}
		return handler(srv, stream)
	}
}

func (i *interceptor) getIP(ctx context.Context) *netip.Addr {
	p, ok := peer.FromContext(ctx)
	if !ok {
		i.LogDebug(ctx, "no peer info in context", extractOperation)
		return nil
	}

	peerAddrPort, err := netip.ParseAddrPort(p.Addr.String())
	if err != nil {
		i.LogError(ctx, "failed to parse peer address", extractOperation, err,
			slog.String("peer", p.Addr.String()),
		)
		return nil
	}

	ip := i.extractor.Extract(ctx, peerAddrPort.Addr(), headers.NewGRPCHeaderGetter(ctx))
	if !ip.IsValid() {
		return nil
	}
	return &ip
}
