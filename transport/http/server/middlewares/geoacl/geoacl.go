// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas-net/netipx"
	"github.com/altessa-s/go-atlas/internal/transport/urlpath"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"
)

const middlewareName = "geoacl"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	resolver              geoacl.GeoResolver
	registry              *geoacl.Registry
	fallbackBehavior      fallback.Behavior
	allowPrivateAddresses bool
}

// Dependencies returns optional middlewares that should run before geoacl.
func (m *middleware) Dependencies() []string {
	return nil
}

// RequiredDependencies returns middlewares that geoacl requires to function.
func (m *middleware) RequiredDependencies() []string {
	return []string{realip.Name()}
}

// New creates a new geographic access control middleware with the given resolver, registry and options.
func New(resolver geoacl.GeoResolver, registry *geoacl.Registry, opt ...Option) *middleware {
	opts := newOptions(opt...)

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		resolver:              resolver,
		registry:              registry,
		fallbackBehavior:      opts.fallbackBehavior,
		allowPrivateAddresses: opts.allowPrivateAddresses,
	}
}

// Middleware returns an HTTP middleware function that enforces geographic access control.
func Middleware(resolver geoacl.GeoResolver, registry *geoacl.Registry, opt ...Option) func(http.Handler) http.Handler {
	return New(resolver, registry, opt...).Handler
}

// Handler wraps an http.Handler with geographic access control.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		ctx := r.Context()

		if m.ShouldIgnoreRequest(r) {
			m.LogIgnored(ctx, path)
			next.ServeHTTP(w, r)
			return
		}

		// A rule is looked up for the path the router dispatches on; any other
		// spelling of it ("//admin", "/public/../admin", "/v1%2Fadmin") is refused.
		if !urlpath.IsCanonicalURL(r.URL) {
			m.LogDebug(ctx, "non-canonical path", path,
				slog.String("method", r.Method))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ip := clientip.FromContext(ctx)
		if !ip.IsValid() {
			m.LogDebug(ctx, "no valid client IP", path,
				slog.String("method", r.Method))
			switch m.fallbackBehavior {
			case fallback.Allow:
				next.ServeHTTP(w, r)
			default:
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
			return
		}

		if m.allowPrivateAddresses && netipx.IsPrivate(ip) {
			m.LogDebug(ctx, "private client address exempt", path,
				slog.String("method", r.Method))
			next.ServeHTTP(w, r)
			return
		}

		allowed, err := m.registry.Evaluate(ctx, m.resolver, ip, path)
		if err != nil {
			m.LogDebug(ctx, "geo resolver error", path,
				slog.String("method", r.Method),
				slog.String("error", err.Error()))
			switch m.fallbackBehavior {
			case fallback.Allow:
				next.ServeHTTP(w, r)
			case fallback.Error:
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			default:
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
			return
		}

		if !allowed {
			m.LogDebug(ctx, "access denied", path,
				slog.String("method", r.Method))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		m.LogDebug(ctx, "access granted", path,
			slog.String("method", r.Method))
		next.ServeHTTP(w, r)
	})
}
