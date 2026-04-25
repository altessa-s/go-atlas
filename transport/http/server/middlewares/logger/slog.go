// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"
	"net/http"

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
	return LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		level := httpStatusToLevel(statusCode)

		attrs := slogx.FieldsToAttrs(fields.Unique())

		l.LogAttrs(ctx, level, msg, attrs...)
	})
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
