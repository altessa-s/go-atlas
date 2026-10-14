// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"
	"net/http"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// LogHandler is the logging backend interface for the logger middleware.
// Implementations receive the request context, a message, the HTTP status
// code, and structured fields collected during request processing.
// The built-in adapter [Slog] wraps a [slog.Logger]; implement this
// interface for custom logging backends. Pass instances to [New] or
// [Middleware].
type LogHandler interface {
	Log(ctx context.Context, msg string, statusCode int, fields slogx.Fields)
}

// LogHandlerFunc is a function adapter that implements [LogHandler].
type LogHandlerFunc func(ctx context.Context, msg string, statusCode int, fields slogx.Fields)

// Log implements LogHandler.
func (f LogHandlerFunc) Log(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
	f(ctx, msg, statusCode, fields)
}

// Slog creates a [LogHandler] backed by a [slog.Logger] for use with
// [New] or [Middleware]. The log level is derived from the HTTP status code:
// 5xx and 401/403 produce Error, 4xx produce Warn, and all others produce
// Info. Fields are converted to slog attributes with dot-notation grouping.
func Slog(l *slog.Logger) LogHandler {
	return SlogWithLevels(l, nil)
}

// SlogWithLevels is [Slog] with per-status level overrides: a status code in
// levels logs at its level (e.g. 401 at Info for a login page), any other at
// Slog's default.
func SlogWithLevels(l *slog.Logger, levels map[int]slog.Level) LogHandler {
	return &slogHandler{logger: l, overrides: coremaps.NewImmutableMap(levels)}
}

// levelChecker is an optional [LogHandler] capability: the middleware skips
// building the completion record when the handler would drop it.
type levelChecker interface {
	Enabled(ctx context.Context, statusCode int) bool
}

// slogHandler is the [LogHandler] behind [Slog] and [SlogWithLevels].
type slogHandler struct {
	logger    *slog.Logger
	overrides *coremaps.ImmutableMap[int, slog.Level]
}

func (h *slogHandler) level(statusCode int) slog.Level {
	if level, ok := h.overrides.Get(statusCode); ok {
		return level
	}
	return httpStatusToLevel(statusCode)
}

// Enabled reports whether a record for statusCode would be logged.
func (h *slogHandler) Enabled(ctx context.Context, statusCode int) bool {
	return h.logger.Enabled(ctx, h.level(statusCode))
}

// Log implements [LogHandler].
func (h *slogHandler) Log(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
	level := h.level(statusCode)
	if !h.logger.Enabled(ctx, level) {
		return
	}
	h.logger.LogAttrs(ctx, level, msg, slogx.FieldsToAttrs(fields.Unique())...)
}

// Matches the logic used in gRPC logger for consistency.
func httpStatusToLevel(statusCode int) slog.Level {
	switch {
	case statusCode >= http.StatusInternalServerError:
		return slog.LevelError
	case statusCode == http.StatusUnauthorized, statusCode == http.StatusForbidden:
		return slog.LevelError
	case statusCode >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
