// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

const middlewareName = "realip"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	extractor *clientip.Extractor
	opts      *options
}

// Dependencies returns middlewares that realip requires to run before it.
// Realip has no dependencies.
func (m *middleware) Dependencies() []string { return nil }

// New creates a new real IP extraction middleware.
// Extractor diagnostics are controlled by the logger used to build extractor.
func New(extractor *clientip.Extractor, logger *slog.Logger, opt ...Option) *middleware {
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, logger),
		extractor:      extractor,
		opts:           newOptions(opt...),
	}
}

// Middleware returns an HTTP middleware that extracts the real client IP address
// from incoming requests and stores it in the request context.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(extractor *clientip.Extractor, logger *slog.Logger, opt ...Option) func(http.Handler) http.Handler {
	return New(extractor, logger, opt...).Handler
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
		attrs := []slog.Attr{slog.String("classification", diagnostics.ErrorClass(err))}
		if m.opts.logClientDetails {
			attrs = append(attrs, slog.String("remote_addr", r.RemoteAddr))
		}
		if !m.opts.logRawErrors {
			err = nil
		}
		m.LogError(r.Context(), "failed to parse remote address", router.RouteTemplate(r), diagnostics.LogError(err), attrs...)
		return netip.Addr{}
	}

	return m.extractor.Extract(r.Context(), peerIP, httpx.NewHTTPHeaderGetter(r))
}
