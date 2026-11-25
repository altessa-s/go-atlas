// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/base"
	"github.com/altessa-s/go-atlas/internal/transport/urlpath"
)

// BaseMiddleware provides common functionality for HTTP middlewares.
// It handles endpoint filtering (ignore patterns), logging, and name
// identification. All methods are safe for concurrent use after construction.
//
// Embed this struct in your middleware implementation to get common functionality:
//
//	type myMiddleware struct {
//	    middlewares.BaseMiddleware
//	    // ... your fields
//	}
//
// For middlewares without ignore patterns:
//
//	func NewMyMiddleware(logger *slog.Logger) *myMiddleware {
//	    return &myMiddleware{
//	        BaseMiddleware: middlewares.NewBaseMiddleware("mymiddleware", logger),
//	    }
//	}
//
// For middlewares with ignore patterns:
//
//	func NewMyMiddleware(ignorePaths []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) *myMiddleware {
//	    return &myMiddleware{
//	        BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter("mymiddleware", ignorePaths, ignorePatterns, logger),
//	    }
//	}
type BaseMiddleware struct {
	// base is not embedded: its path-only ShouldIgnore must not be reachable
	// from a handler, see ShouldIgnoreRequest.
	base base.Base
}

// Name returns the middleware name.
func (b *BaseMiddleware) Name() string { return b.base.Name() }

// Logger returns the configured slog logger.
func (b *BaseMiddleware) Logger() *slog.Logger { return b.base.Logger() }

// LogIgnored logs a debug message that the path was ignored.
func (b *BaseMiddleware) LogIgnored(ctx context.Context, path string) { b.base.LogIgnored(ctx, path) }

// LogDebug logs a debug message with middleware context.
func (b *BaseMiddleware) LogDebug(ctx context.Context, msg, path string, attrs ...slog.Attr) {
	b.base.LogDebug(ctx, msg, path, attrs...)
}

// LogWarn logs a warning message with middleware context.
func (b *BaseMiddleware) LogWarn(ctx context.Context, msg, path string, err error, attrs ...slog.Attr) {
	b.base.LogWarn(ctx, msg, path, diagnostics.LogError(err), attrs...)
}

// LogError logs an error message with middleware context.
func (b *BaseMiddleware) LogError(ctx context.Context, msg, path string, err error, attrs ...slog.Attr) {
	b.base.LogError(ctx, msg, path, diagnostics.LogError(err), attrs...)
}

// NewBaseMiddleware creates a new BaseMiddleware with the given name and logger.
// This is the basic constructor for middlewares that don't need path filtering.
//
// Parameters:
//   - name: The middleware name used for identification
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseMiddleware(name string, logger *slog.Logger) BaseMiddleware {
	return BaseMiddleware{base: base.New(name, "middleware", "path", logger)}
}

// NewBaseMiddlewareWithFilter creates a new BaseMiddleware with path filtering support.
// Use this constructor for middlewares that need to skip certain paths.
//
// Parameters:
//   - name: The middleware name used for identification
//   - ignorePaths: List of path names to skip (e.g., "/health", "/metrics")
//   - ignorePatterns: Regex patterns for paths to skip
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseMiddlewareWithFilter(name string, ignorePaths []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) BaseMiddleware {
	return BaseMiddleware{base: base.NewWithFilter(name, "middleware", "path", ignorePaths, ignorePatterns, logger)}
}

// ShouldIgnoreRequest reports whether the request path is on the ignore list.
// Only a canonical request path ([urlpath.IsCanonicalURL]) can be:
// "/healthz/../admin", "//healthz" or "/v1%2Fhealthz" never skips the
// middleware, whatever the router behind it makes of them. It is the only way
// to consult the list: a bare path does not tell how it was escaped.
func (b *BaseMiddleware) ShouldIgnoreRequest(r *http.Request) bool {
	return urlpath.IsCanonicalURL(r.URL) && b.base.ShouldIgnore(r.URL.Path)
}

// WrapResponseWriter wraps an [http.ResponseWriter] to capture status code and body.
// Callers must call [ResponseWriter.Release] on the returned value when finished
// to return it to the pool. This is a convenience wrapper around [NewResponseWriter].
func (b *BaseMiddleware) WrapResponseWriter(w http.ResponseWriter, captureBody bool) *ResponseWriter {
	return NewResponseWriter(w, captureBody)
}

// BoundedMethod maps r.Method to a bounded observability value. net/http accepts any
// token as a method, so an unauthenticated client could otherwise mint a new
// metric series per request.
func BoundedMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}
