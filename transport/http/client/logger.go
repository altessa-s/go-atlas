// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"cmp"
	"context"
	"log/slog"
	"time"
)

// DefaultLogTimeout is the default timeout for log operations.
const DefaultLogTimeout = 1 * time.Second

// LeveledLogger is the interface expected by hashicorp/go-retryablehttp for
// emitting retry-related log messages. [NewLogger] bridges an [slog.Logger]
// to this interface.
type LeveledLogger interface {
	Error(string, ...any)
	Info(string, ...any)
	Debug(string, ...any)
	Warn(string, ...any)
}

// Logger adapts an [slog.Logger] to the [LeveledLogger] interface required by
// go-retryablehttp. Each log call creates a short-lived context with a deadline
// (default [DefaultLogTimeout]) so that slow slog handlers (e.g., those writing
// over the network) do not block the HTTP client indefinitely.
//
// Logger is safe for concurrent use because the underlying [slog.Logger] is
// itself safe for concurrent use. Create instances with [NewLogger].
type Logger struct {
	logger  *slog.Logger
	timeout time.Duration
}

// LoggerOption configures the Logger.
type LoggerOption func(*Logger)

// WithLogTimeout sets the timeout for log operations.
// This is useful when slog handlers perform network or database operations.
// Default is 1 second.
//
// Example:
//
//	logger := httpclient.NewLogger(slog.Default(), httpclient.WithLogTimeout(2*time.Second))
func WithLogTimeout(timeout time.Duration) LoggerOption {
	return func(l *Logger) {
		l.timeout = timeout
	}
}

// NewLogger creates a new [Logger] adapter that wraps an [slog.Logger] to implement
// the [LeveledLogger] interface required by go-retryablehttp.
// If logger is nil, a discard handler is used.
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	leveledLogger := httpclient.NewLogger(logger)
//
// Example with timeout:
//
//	leveledLogger := httpclient.NewLogger(logger, httpclient.WithLogTimeout(2*time.Second))
func NewLogger(logger *slog.Logger, opts ...LoggerOption) LeveledLogger {
	l := Logger{
		logger:  cmp.Or(logger, slog.New(slog.DiscardHandler)),
		timeout: DefaultLogTimeout,
	}
	for _, opt := range opts {
		opt(&l)
	}
	return l
}

func (l Logger) logContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), l.timeout) //nolint:contextcheck // external library interface doesn't support context
}

// Error logs a message at error level.
func (l Logger) Error(msg string, args ...any) {
	if l.logger == nil {
		return
	}
	ctx, cancel := l.logContext()
	defer cancel()
	l.logger.ErrorContext(ctx, msg, args...)
}

// Warn logs a message at warning level.
func (l Logger) Warn(msg string, args ...any) {
	if l.logger == nil {
		return
	}
	ctx, cancel := l.logContext()
	defer cancel()
	l.logger.WarnContext(ctx, msg, args...)
}

// Debug logs a message at debug level.
func (l Logger) Debug(msg string, args ...any) {
	if l.logger == nil {
		return
	}
	ctx, cancel := l.logContext()
	defer cancel()
	l.logger.DebugContext(ctx, msg, args...)
}

// Info logs a message at info level.
func (l Logger) Info(msg string, args ...any) {
	if l.logger == nil {
		return
	}
	ctx, cancel := l.logContext()
	defer cancel()
	l.logger.InfoContext(ctx, msg, args...)
}
