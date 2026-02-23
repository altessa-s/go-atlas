// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/headers"
)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	extractor *clientip.Extractor
}

// Dependencies returns middlewares that realip requires to run before it.
// Realip has no dependencies.
func (m *middleware) Dependencies() []string { return nil }

// New creates a new real IP extraction middleware.
func New(extractor *clientip.Extractor, logger *slog.Logger) *middleware {
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware("realip", logger),
		extractor:      extractor,
	}
}

// Middleware returns an HTTP middleware that extracts the real client IP address
// from incoming requests and stores it in the request context.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(extractor *clientip.Extractor, logger *slog.Logger) func(http.Handler) http.Handler {
	return New(extractor, logger).Handler
}

// Handler wraps an http.Handler with real IP extraction functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := m.getIP(r); ip.IsValid() {
			r = r.WithContext(clientip.NewContext(r.Context(), ip))
		}
		next.ServeHTTP(w, r)
	})
}

func (m *middleware) getIP(r *http.Request) netip.Addr {
	// Parse remote address
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Try parsing as just an IP (no port)
		remoteHost = r.RemoteAddr
	}

	peerIP, err := netip.ParseAddr(remoteHost)
	if err != nil {
		m.LogError(r.Context(), "failed to parse remote address", r.URL.Path, err,
			slog.String("remote_addr", r.RemoteAddr),
		)
		return netip.Addr{}
	}

	return m.extractor.Extract(r.Context(), peerIP, headers.NewHTTPHeaderGetter(r))
}

// FromContext returns the real IP address stored by this middleware from the
// context. Returns an invalid [netip.Addr] (where IsValid returns false) if
// no real IP is present. The value is set by [middleware.Handler] or
// manually via [NewContext].
func FromContext(ctx context.Context) netip.Addr {
	return clientip.FromContext(ctx)
}

// NewContext returns a new context with the given real IP address.
// This is useful for injecting a real IP into the context manually,
// for example in tests. Retrieve it later with [FromContext].
func NewContext(ctx context.Context, ip netip.Addr) context.Context {
	return clientip.NewContext(ctx, ip)
}
