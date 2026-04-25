// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slog

import (
	"context"
	"log/slog"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

type ctxSlog struct{}

// ctxSlogKey is the key used to store the logger in the context.
var ctxSlogKey = &ctxSlog{}

// ContextWithLogger stores the provided logger in context for retrieval via FromContext.
// Panics if logger is nil. Returns unchanged context if a logger already exists.
//
// Example:
//
//	ctx = slogx.ContextWithLogger(ctx, logger.With("request_id", id))
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		panic("logger cannot be nil")
	}

	// Allow nil ctx for convenience.
	ctx = corecontext.OrBackground(ctx)

	if _, ok := ctx.Value(ctxSlogKey).(*slog.Logger); !ok {
		ctx = context.WithValue(ctx, ctxSlogKey, logger)
	}

	return ctx
}

// FromContext retrieves the logger previously stored via ContextWithLogger.
// Returns nil if no logger has been stored in the context.
//
// Example:
//
//	logger := slogx.FromContext(ctx) // returns nil if not set
func FromContext(ctx context.Context) *slog.Logger {
	l, ok := corecontext.OrBackground(ctx).Value(ctxSlogKey).(*slog.Logger)
	if ok {
		return l
	}
	return nil
}

// FromContextOrDefault returns logger from context or slog.Default() if not found.
// This is the recommended way to get logger in handlers/services.
//
// Example:
//
//	logger := slogx.FromContextOrDefault(ctx)
//	logger.Info("processing", "user_id", userID)
func FromContextOrDefault(ctx context.Context) *slog.Logger {
	if l := FromContext(ctx); l != nil {
		return l
	}
	return slog.Default()
}

// BuildLogger creates a logger enriched with fields from context.
// If no fields in context, returns base logger unchanged.
// If base is nil, uses slog.Default().
//
// Example:
//
//	logger := slogx.BuildLogger(ctx, nil) // uses slog.Default() + context fields
func BuildLogger(ctx context.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	fields := FieldsFromContext(ctx)
	if len(fields) == 0 {
		return base
	}
	return base.With(fields.ToSlogArgs()...)
}

// InjectLogger creates enriched logger from base + context fields and stores in context.
// Combines BuildLogger and ContextWithLogger in one call.
// If the context already has a logger, it will not be replaced.
//
// Example (in interceptor):
//
//	ctx = slogx.InjectLogger(ctx, baseLogger)
func InjectLogger(ctx context.Context, base *slog.Logger) context.Context {
	enriched := BuildLogger(ctx, base)
	return ContextWithLogger(ctx, enriched)
}
