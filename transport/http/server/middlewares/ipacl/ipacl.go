// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

const middlewareName = "ipacl"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	registry         *ipacl.Registry
	fallbackBehavior fallback.Behavior
}

// Dependencies returns optional middlewares that should run before ipacl.
func (m *middleware) Dependencies() []string {
	return nil
}

// RequiredDependencies returns middlewares that ipacl requires to function.
func (m *middleware) RequiredDependencies() []string {
	return []string{realip.Name()}
}

// New creates a new IP access control middleware with the given registry and options.
func New(registry *ipacl.Registry, opt ...Option) *middleware {
	opts := newOptions(opt...)

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		registry:         registry,
		fallbackBehavior: opts.fallbackBehavior,
	}
}

// Middleware returns an HTTP middleware function that enforces IP-based access control.
func Middleware(registry *ipacl.Registry, opt ...Option) func(http.Handler) http.Handler {
	return New(registry, opt...).Handler
}

// Handler wraps an http.Handler with IP access control.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := m.InternPath(r.URL.Path)
		ctx := r.Context()

		if m.ShouldIgnore(path) {
			m.LogIgnored(ctx, path)
			next.ServeHTTP(w, r)
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

		if !m.registry.Evaluate(ip, path) {
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
