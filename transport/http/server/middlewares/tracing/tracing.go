// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"net"
	"net/http"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

const middlewareName = "tracing"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*Middleware)(nil)

// Middleware provides distributed tracing for HTTP servers.
// Create instances with [New]. The [Middleware.Handler] method is safe for
// concurrent use. The middleware declares a dependency on the "realip"
// middleware to extract the client IP for the [NetPeerIPKey] span attribute;
// if realip is absent the attribute is populated from [http.Request.RemoteAddr].
type Middleware struct {
	middlewares.BaseMiddleware
	tracer     tracing.Tracer
	recorder   tracing.Recorder
	propagator propagation.TextMapPropagator
	opts       *options
}

// Dependencies returns middlewares that tracing requires to run before it.
// Tracing uses realip for client IP extraction if available.
func (m *Middleware) Dependencies() []string {
	return []string{realip.Name()}
}

// New creates a new tracing [Middleware] with the provided tracer and options.
// If tracer is nil, a no-op tracer is used, effectively disabling tracing.
func New(tracer tracing.Tracer, opts ...Option) *Middleware {
	if tracer == nil {
		tracer = tracing.Noop()
	}

	cfg := newOptions(opts...)

	return &Middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			cfg.ignorePaths,
			cfg.ignorePatterns,
			cfg.logger,
		),
		tracer:     tracer,
		recorder:   tracer.Recorder("http/server"),
		propagator: cfg.propagator,
		opts:       cfg,
	}
}

// Handler wraps an http.Handler with distributed tracing.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnore(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract trace context from headers
		ctx := m.propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start server span
		spanName := m.opts.spanNameFunc(r)
		ctx, span := m.recorder.Start(ctx, spanName,
			tracing.WithSpanKind(tracing.SpanKindServer),
			tracing.WithAttributes(requestAttributes(r)...),
		)
		defer span.End()

		// Add client IP if available
		if ip := extractClientIP(r); ip != "" {
			span.SetAttributes(tracing.String(NetPeerIPKey, ip))
		}

		// Update request with traced context
		r = r.WithContext(ctx)

		// Wrap response writer to capture status code
		sw := middlewares.NewResponseWriter(w, false)
		defer sw.Release()

		// Call next handler
		next.ServeHTTP(sw, r)

		// Record response attributes
		statusCode := sw.StatusCode()
		span.SetAttributes(tracing.Int(HTTPStatusCodeKey, statusCode))

		// Set span status based on HTTP status code
		if statusCode >= http.StatusBadRequest {
			span.SetStatus(tracing.StatusError, http.StatusText(statusCode))
		} else {
			span.SetStatus(tracing.StatusOK, "")
		}
	})
}

// TracingHandler creates an HTTP middleware that provides distributed tracing.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func TracingHandler(tracer tracing.Tracer, opts ...Option) func(http.Handler) http.Handler {
	return New(tracer, opts...).Handler
}

// WrapHandler wraps an http.Handler with tracing middleware.
// This is a convenience function for wrapping a single handler.
func WrapHandler(tracer tracing.Tracer, handler http.Handler, opts ...Option) http.Handler {
	return TracingHandler(tracer, opts...)(handler)
}

// extractClientIP extracts the client IP from the request.
// It first checks the context for a real IP set by the realip middleware,
// then falls back to parsing the RemoteAddr.
func extractClientIP(r *http.Request) string {
	// Check context for real IP (set by realip middleware)
	if ip := clientip.FromContext(r.Context()); ip.IsValid() {
		return ip.String()
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
