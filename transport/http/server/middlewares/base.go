// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
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
	name          string
	logger        *slog.Logger
	ignoreChecker endpointfilter.Filter
}

// NewBaseMiddleware creates a new BaseMiddleware with the given name and logger.
// This is the basic constructor for middlewares that don't need path filtering.
//
// Parameters:
//   - name: The middleware name used for identification
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseMiddleware(name string, logger *slog.Logger) BaseMiddleware {
	return BaseMiddleware{
		name:          name,
		logger:        cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewNoop(),
	}
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
	return BaseMiddleware{
		name:   name,
		logger: cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewOrNoop(
			ignorePaths,
			endpointfilter.WithIgnorePatterns(ignorePatterns...),
		),
	}
}

// Name returns the middleware name.
func (b *BaseMiddleware) Name() string {
	return b.name
}

// Logger returns the configured slog logger.
func (b *BaseMiddleware) Logger() *slog.Logger {
	return b.logger
}

// ShouldIgnore checks if the given path should be ignored based on configured patterns.
// The path is automatically lowercased and interned for efficient comparison.
func (b *BaseMiddleware) ShouldIgnore(path string) bool {
	return b.ignoreChecker.ShouldFilter(strings.InternLowerString(path))
}

// LogIgnored logs a debug message that the path was ignored.
func (b *BaseMiddleware) LogIgnored(ctx context.Context, path string) {
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, "ignored",
		slog.String("middleware", b.name),
		slog.String("path", path),
	)
}

// LogDebug logs a debug message with middleware context.
func (b *BaseMiddleware) LogDebug(ctx context.Context, msg string, path string, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+2)
	allAttrs = append(allAttrs, slog.String("middleware", b.name), slog.String("path", path))
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, msg, allAttrs...)
}

// LogWarn logs a warning message with middleware context.
func (b *BaseMiddleware) LogWarn(ctx context.Context, msg string, path string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String("middleware", b.name), slog.String("path", path))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", err))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelWarn, msg, allAttrs...)
}

// LogError logs an error message with middleware context.
func (b *BaseMiddleware) LogError(ctx context.Context, msg string, path string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String("middleware", b.name), slog.String("path", path))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", err))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelError, msg, allAttrs...)
}

// InternPath returns an interned version of the path for memory efficiency.
func (b *BaseMiddleware) InternPath(path string) string {
	return strings.InternString(path)
}

// WrapResponseWriter wraps an [http.ResponseWriter] to capture status code and body.
// Callers must call [ResponseWriter.Release] on the returned value when finished
// to return it to the pool. This is a convenience wrapper around [NewResponseWriter].
func (b *BaseMiddleware) WrapResponseWriter(w http.ResponseWriter, captureBody bool) *ResponseWriter {
	return NewResponseWriter(w, captureBody)
}
