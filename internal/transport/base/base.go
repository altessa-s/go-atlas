// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"cmp"
	"context"
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/endpointfilter"

	slogx "github.com/altessa-s/go-slogx"
)

// Base provides common functionality shared by HTTP middlewares and gRPC
// interceptors: name identification, endpoint filtering (ignore patterns),
// string interning, and structured logging with parameterized attribute keys.
//
// The componentKind and endpointKind fields control the slog attribute keys
// used in log methods, so log output is identical to the original per-transport
// implementations (e.g. "middleware"/"path" for HTTP, "interceptor"/"method"
// for gRPC).
//
// All methods are safe for concurrent use after construction.
type Base struct {
	name          string
	logger        *slog.Logger
	ignoreChecker endpointfilter.Filter
	componentKind string // e.g. "middleware", "interceptor"
	endpointKind  string // e.g. "path", "method"
}

// New creates a Base with the given name and logger, without endpoint filtering.
//
// Parameters:
//   - name: The component name used for identification and ordering
//   - componentKind: The slog attribute key for the component (e.g. "middleware", "interceptor")
//   - endpointKind: The slog attribute key for the endpoint (e.g. "path", "method")
//   - logger: The slog.Logger for structured logging (can be nil for no logging)
func New(name, componentKind, endpointKind string, logger *slog.Logger) Base {
	return Base{
		name:          name,
		logger:        cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewNoop(),
		componentKind: componentKind,
		endpointKind:  endpointKind,
	}
}

// NewWithFilter creates a Base with endpoint filtering support.
//
// Parameters:
//   - name: The component name used for identification and ordering
//   - componentKind: The slog attribute key for the component (e.g. "middleware", "interceptor")
//   - endpointKind: The slog attribute key for the endpoint (e.g. "path", "method")
//   - endpoints: List of endpoint names to skip (e.g. "/health", "/grpc.health.v1.Health/Check")
//   - patterns: Regex patterns for endpoints to skip
//   - logger: The slog.Logger for structured logging (can be nil for no logging)
func NewWithFilter(name, componentKind, endpointKind string, endpoints []string, patterns []*regexp.Regexp, logger *slog.Logger) Base {
	return Base{
		name:   name,
		logger: cmp.Or(logger, slog.New(slog.DiscardHandler)),
		ignoreChecker: endpointfilter.NewOrNoop(
			endpoints,
			endpointfilter.WithIgnorePatterns(patterns...),
			endpointfilter.WithFoldCase(),
		),
		componentKind: componentKind,
		endpointKind:  endpointKind,
	}
}

// Name returns the component name.
func (b *Base) Name() string {
	return b.name
}

// Logger returns the configured slog logger.
func (b *Base) Logger() *slog.Logger {
	return b.logger
}

// ShouldIgnore checks if the given endpoint should be ignored based on
// configured endpoints and patterns. Matching is case-insensitive: the
// patterns see the lowercased endpoint. The check takes no lock and does not
// intern the request-derived endpoint.
func (b *Base) ShouldIgnore(endpoint string) bool {
	return b.ignoreChecker.ShouldFilter(endpoint)
}

// LogIgnored logs a debug message that the endpoint was ignored.
func (b *Base) LogIgnored(ctx context.Context, endpoint string) {
	if !b.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, "ignored",
		slog.String(b.componentKind, b.name),
		slog.String(b.endpointKind, endpoint),
	)
}

// LogDebug logs a debug message with component context.
func (b *Base) LogDebug(ctx context.Context, msg, endpoint string, attrs ...slog.Attr) {
	if !b.logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	allAttrs := make([]slog.Attr, 0, len(attrs)+2)
	allAttrs = append(allAttrs, slog.String(b.componentKind, b.name), slog.String(b.endpointKind, endpoint))
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelDebug, msg, allAttrs...)
}

// LogWarn logs a warning message with component context.
func (b *Base) LogWarn(ctx context.Context, msg, endpoint string, err error, attrs ...slog.Attr) {
	if !b.logger.Enabled(ctx, slog.LevelWarn) {
		return
	}
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String(b.componentKind, b.name), slog.String(b.endpointKind, endpoint))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", diagnostics.LogError(err)))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelWarn, msg, allAttrs...)
}

// LogError logs an error message with component context.
func (b *Base) LogError(ctx context.Context, msg, endpoint string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+3)
	allAttrs = append(allAttrs, slog.String(b.componentKind, b.name), slog.String(b.endpointKind, endpoint))
	if err != nil {
		allAttrs = append(allAttrs, slog.Any("error", diagnostics.LogError(err)))
	}
	allAttrs = append(allAttrs, attrs...)
	slogx.BuildLogger(ctx, b.logger).LogAttrs(ctx, slog.LevelError, msg, allAttrs...)
}
