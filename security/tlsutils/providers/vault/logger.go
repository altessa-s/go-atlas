// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault

import (
	"cmp"
	"context"
	"log/slog"
	"sync"
)

// Logger is the Vault logger implementation.
// It adapts slog.Logger to the certify logger interface.
type Logger struct {
	logger *slog.Logger
	pool   *attrPool
	ctx    context.Context
}

// attrPool provides a pool for attribute slices
type attrPool struct {
	pool sync.Pool
}

func newAttrPool() *attrPool {
	return &attrPool{
		pool: sync.Pool{
			New: func() any {
				attrs := make([]slog.Attr, 0, 8)
				return &attrs
			},
		},
	}
}

func (p *attrPool) get() *[]slog.Attr {
	attrs, ok := p.pool.Get().(*[]slog.Attr)
	if !ok {
		// Should never happen if pool is used correctly
		slice := make([]slog.Attr, 0, 10)
		return &slice
	}
	return attrs
}

func (p *attrPool) put(attrs *[]slog.Attr) {
	*attrs = (*attrs)[:0]
	p.pool.Put(attrs)
}

var globalAttrPool = newAttrPool()

// NewLogger creates a new logger instance.
// If logger is nil, slog.Default() is used.
//
// Example:
//
//	logger := NewLogger(slog.Default())
//
//nolint:contextcheck // ctx is a parameter; we intentionally accept nil and fall back to Background for library ergonomics.
func NewLogger(logger *slog.Logger) *Logger {
	return NewLoggerWithContext(context.Background(), logger)
}

// NewLoggerWithContext creates a new logger instance that uses ctx for slog operations.
// If logger is nil, slog.Default() is used.
// If ctx is nil, context.Background() is used.
//
//nolint:contextcheck // ctx is a parameter; we intentionally accept nil and fall back to Background for library ergonomics.
func NewLoggerWithContext(ctx context.Context, logger *slog.Logger) *Logger {
	return &Logger{
		logger: cmp.Or(logger, slog.Default()),
		pool:   globalAttrPool,
		ctx:    cmp.Or(ctx, context.Background()),
	}
}

// Trace logs a trace message.
// Trace level is mapped to Debug in slog.
func (l *Logger) Trace(msg string, fields ...map[string]any) {
	l.logWithLevel(slog.LevelDebug, msg, fields...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]any) {
	l.logWithLevel(slog.LevelDebug, msg, fields...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]any) {
	l.logWithLevel(slog.LevelInfo, msg, fields...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]any) {
	l.logWithLevel(slog.LevelWarn, msg, fields...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]any) {
	l.logWithLevel(slog.LevelError, msg, fields...)
}

// logWithLevel is an optimized logging method that minimizes allocations
func (l *Logger) logWithLevel(level slog.Level, msg string, fields ...map[string]any) {
	ctx := cmp.Or(l.ctx, context.Background())

	if l.logger == nil || !l.logger.Enabled(ctx, level) {
		return
	}

	// Fast path for no fields
	if len(fields) == 0 {
		l.logger.Log(ctx, level, msg)
		return
	}

	// Get attributes from pool
	attrsPtr := l.pool.get()
	defer l.pool.put(attrsPtr)
	attrs := *attrsPtr

	// Convert fields to attributes using slog.Any for better performance
	for _, field := range fields {
		for k, v := range field {
			attrs = append(attrs, slog.Any(k, v))
		}
	}

	l.logger.LogAttrs(ctx, level, msg, attrs...)
}
