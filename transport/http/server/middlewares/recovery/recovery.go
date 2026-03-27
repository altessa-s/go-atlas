// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas/core/runtime/helpers"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/recovery"
)

const logPanicStackSkip = 2

const middlewareName = "recovery"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	handler  PanicHandler
	logStack bool
}

// Dependencies returns middlewares that recovery reads from context.
// All dependencies are optional for ordering - recovery gracefully degrades
// if requestid is not available in context.
func (m *middleware) Dependencies() []string {
	return []string{requestid.Name()}
}

// New creates a new panic recovery middleware.
// The logger is required for panic reporting; pass additional [Option]
// values to customize the handler and stack-trace logging.
func New(logger *slog.Logger, opt ...Option) *middleware {
	allOpts := append([]Option{WithLogger(logger)}, opt...)
	opts := newOptions(allOpts...)

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		handler:  opts.handler,
		logStack: opts.logStack,
	}
}

// PanicRecover returns middleware that recovers from panics.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func PanicRecover(logger *slog.Logger, opt ...Option) func(next http.Handler) http.Handler {
	return New(logger, opt...).Handler
}

// Handler wraps an http.Handler with panic recovery functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnore(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		// Create panic handling options
		panicOpts := panics.NewHandleOpts().SetReallyPanic(false)

		// Create a handler that captures panic for logging
		httpPanicHandler := func(ctx context.Context, p any) {
			// Check for http.ErrAbortHandler - don't handle it
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(err)
			}

			// Log the panic using observability logger
			m.logPanic(ctx, request, p)

			// Call custom handler if provided
			if m.handler != nil {
				m.handler(ctx, writer, request, p)
			} else {
				// Default handler: return 500
				writer.WriteHeader(http.StatusInternalServerError)
			}
		}

		defer panics.HandleWithOpts(request.Context(), panicOpts, httpPanicHandler)

		next.ServeHTTP(writer, request)
	})
}

// logPanic logs the panic with request context including request ID, stack trace, and goroutine ID.
// This matches the logging format used in gRPC recovery interceptor.
func (m *middleware) logPanic(ctx context.Context, request *http.Request, p any) {
	// Get request ID from context
	requestID := requestid.FromContext(ctx)

	attrs := []slog.Attr{
		slog.String("method", request.Method),
		slog.String("request_id", requestID),
		slog.Any("panic", p),
		slog.Int("goroutine_id", helpers.GoroutineID()),
	}

	// Optionally include stack trace
	if m.logStack {
		frames := recovery.StackTrace(logPanicStackSkip)
		attrs = append(attrs, slog.Any("stack_trace", frames))
	}

	m.LogError(ctx, "panic recovered", request.URL.Path, nil, attrs...)
}
