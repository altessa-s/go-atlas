// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/internal/base"
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
	base.Base
}

// NewBaseMiddleware creates a new BaseMiddleware with the given name and logger.
// This is the basic constructor for middlewares that don't need path filtering.
//
// Parameters:
//   - name: The middleware name used for identification
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseMiddleware(name string, logger *slog.Logger) BaseMiddleware {
	return BaseMiddleware{Base: base.New(name, "middleware", "path", logger)}
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
	return BaseMiddleware{Base: base.NewWithFilter(name, "middleware", "path", ignorePaths, ignorePatterns, logger)}
}

// InternPath returns an interned version of the path for memory efficiency.
func (b *BaseMiddleware) InternPath(path string) string {
	return b.InternEndpoint(path)
}

// WrapResponseWriter wraps an [http.ResponseWriter] to capture status code and body.
// Callers must call [ResponseWriter.Release] on the returned value when finished
// to return it to the pool. This is a convenience wrapper around [NewResponseWriter].
func (b *BaseMiddleware) WrapResponseWriter(w http.ResponseWriter, captureBody bool) *ResponseWriter {
	return NewResponseWriter(w, captureBody)
}
