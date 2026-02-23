// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"cmp"
	"context"
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// BaseInterceptor provides common functionality for gRPC interceptors.
// It handles endpoint filtering (ignore patterns), logging, and name identification.
//
// Embed this struct in your interceptor implementation to get common functionality:
//
//	type myInterceptor struct {
//	    interceptors.BaseInterceptor
//	    // ... your fields
//	}
//
// For interceptors without ignore patterns:
//
//	func NewMyInterceptor(logger *slog.Logger) *myInterceptor {
//	    return &myInterceptor{
//	        BaseInterceptor: interceptors.NewBaseInterceptor("myinterceptor", logger),
//	    }
//	}
//
// For interceptors with ignore patterns:
//
//	func NewMyInterceptor(ignoreMethods []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) *myInterceptor {
//	    return &myInterceptor{
//	        BaseInterceptor: interceptors.NewBaseInterceptorWithFilter("myinterceptor", ignoreMethods, ignorePatterns, logger),
//	    }
//	}
type BaseInterceptor struct {
	name          string
	logger        *slog.Logger
	ignoreChecker endpointfilter.Filter
}

// NewBaseInterceptor creates a new BaseInterceptor with the given name and logger.
// This is the basic constructor for interceptors that don't need method filtering.
//
// Parameters:
//   - name: The interceptor name used for identification and ordering
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseInterceptor(name string, logger *slog.Logger) BaseInterceptor {
	return BaseInterceptor{
		name:          name,
		logger:        cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewNoop(),
	}
}

// NewBaseInterceptorWithFilter creates a new BaseInterceptor with method filtering support.
// Use this constructor for interceptors that need to skip certain methods.
//
// Parameters:
//   - name: The interceptor name used for identification and ordering
//   - ignoreMethods: List of method names to skip (e.g., "/grpc.health.v1.Health/Check")
//   - ignorePatterns: Regex patterns for methods to skip
//   - logger: The slog.Logger for debug/info/error logging (can be nil for no logging)
func NewBaseInterceptorWithFilter(name string, ignoreMethods []string, ignorePatterns []*regexp.Regexp, logger *slog.Logger) BaseInterceptor {
	return BaseInterceptor{
		name:   name,
		logger: cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewOrNoop(
			ignoreMethods,
			endpointfilter.WithIgnorePatterns(ignorePatterns...),
		),
	}
}

// Name returns the interceptor name.
func (b *BaseInterceptor) Name() string {
	return b.name
}

// Logger returns the configured slog logger.
func (b *BaseInterceptor) Logger() *slog.Logger {
	return b.logger
}

// ShouldIgnore checks if the given method should be ignored based on configured patterns.
// The method name is automatically lowercased and interned for efficient comparison.
func (b *BaseInterceptor) ShouldIgnore(method string) bool {
	return b.ignoreChecker.ShouldFilter(strings.InternLowerString(method))
}

// ShouldIgnoreFromContext checks if the current call should be ignored based on CallMetadata.
// Returns true if the method should be skipped, along with the CallMetadata.
func (b *BaseInterceptor) ShouldIgnoreFromContext(ctx context.Context) (*metadata.CallMetadata, bool) {
	meta, ok := metadata.FromContext(ctx)
	if !ok {
		return nil, false
	}
	return meta, b.ShouldIgnore(meta.FullyMethodName)
}

// LogIgnored logs a debug message that the method was ignored.
func (b *BaseInterceptor) LogIgnored(ctx context.Context, method string) {
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, "ignored",
		slog.String("interceptor", b.name),
		slog.String("method", method),
	)
}

// LogDebug logs a debug message with interceptor context.
func (b *BaseInterceptor) LogDebug(ctx context.Context, msg string, method string, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+2)
	allAttrs = append(allAttrs, slog.String("interceptor", b.name), slog.String("method", method))
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, msg, allAttrs...)
}

// LogWarn logs a warning message with interceptor context.
func (b *BaseInterceptor) LogWarn(ctx context.Context, msg string, method string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String("interceptor", b.name), slog.String("method", method))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", err))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelWarn, msg, allAttrs...)
}

// LogError logs an error message with interceptor context.
func (b *BaseInterceptor) LogError(ctx context.Context, msg string, method string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String("interceptor", b.name), slog.String("method", method))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", err))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelError, msg, allAttrs...)
}

// InternMethod returns an interned version of the method name for memory efficiency.
func (b *BaseInterceptor) InternMethod(method string) string {
	return strings.InternString(method)
}
