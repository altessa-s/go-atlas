// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas-core/runtime/helpers"
	"github.com/altessa-s/go-atlas/internal/transport/recovery"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/internal/requestidcapture"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

const logPanicStackSkip = 1

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
	handler      PanicHandler
	logRawErrors bool
}

// Outermost keeps panics in other middleware within the recovery boundary.
func (*middleware) Outermost() bool { return true }

// Dependencies reports that recovery can run before every other middleware.
func (*middleware) Dependencies() []string { return nil }

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
		handler:      opts.handler,
		logRawErrors: opts.logRawErrors,
	}
}

// PanicRecover returns middleware that recovers from panics.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func PanicRecover(logger *slog.Logger, opt ...Option) func(next http.Handler) http.Handler {
	return New(logger, opt...).Handler
}

// Handler wraps an http.Handler with panic recovery functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	observeID := m.Logger().Handler() != slog.DiscardHandler
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnoreRequest(request) {
			next.ServeHTTP(writer, request)
			return
		}

		// Track whether the response has started, for HeadersWritten and
		// the default handler.
		tracked := &trackingWriter{ResponseWriter: writer}

		// Inner middleware passes a new request to its children. Observe the ID
		// it resolves even if those children replace or commit response headers.
		if observeID {
			request = request.WithContext(requestidcapture.WithCapture(request.Context()))
		}

		defer func(ctx context.Context) {
			p := recover()
			if p == nil {
				return
			}
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(http.ErrAbortHandler)
			}

			// Log the panic using observability logger
			id := cmp.Or(requestidcapture.ID(ctx), requestid.FromContext(ctx))
			m.logPanic(ctx, request, p, id)

			// Call custom handler if provided
			if m.handler != nil {
				m.handler(ctx, tracked, request, p)
			} else if !tracked.started {
				tracked.WriteHeader(http.StatusInternalServerError)
			}
		}(request.Context())

		next.ServeHTTP(tracked, request)
	})
}

// logPanic logs the panic with request context including request ID, stack trace, and goroutine ID.
// This matches the logging format used in gRPC recovery interceptor.
func (m *middleware) logPanic(ctx context.Context, request *http.Request, p any, requestID string) {
	attrs := []slog.Attr{
		slog.String("method", middlewares.BoundedMethod(request.Method)),
		slog.String("request_id", requestID),
		slog.Int("goroutine_id", helpers.GoroutineID()),
	}

	// Optionally include stack trace
	if m.logRawErrors {
		frames := recovery.StackTrace(logPanicStackSkip)
		attrs = append(attrs, slog.Any("panic", p), slog.Any("stack_trace", frames))
	}

	m.LogError(ctx, "panic recovered", router.RouteTemplate(request), nil, attrs...)
}
