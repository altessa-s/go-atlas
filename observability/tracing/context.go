// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey struct{}

// spanKey is the context key for storing the current span.
var spanKey = contextKey{}

// SpanFromContext returns the Span stored in ctx.
// If no span is stored, it returns a no-op span that does nothing.
// This ensures it's always safe to call methods on the returned span.
//
// Example:
//
//	span := tracing.SpanFromContext(ctx)
//	span.SetAttributes(tracing.String("key", "value"))
func SpanFromContext(ctx context.Context) Span {
	if span, ok := corecontext.OrBackground(ctx).Value(spanKey).(Span); ok && span != nil {
		return span
	}
	return noopSpan{}
}

// ContextWithSpan returns a new context with the given span stored.
// Use this when you need to manually propagate a span to child operations.
//
// Example:
//
//	ctx = tracing.ContextWithSpan(ctx, span)
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	ctx = corecontext.OrBackground(ctx)
	return context.WithValue(ctx, spanKey, span)
}

// SpanContextFromContext returns the SpanContext from the span in ctx.
// If no span is stored, it returns an invalid SpanContext.
//
// Example:
//
//	sc := tracing.SpanContextFromContext(ctx)
//	if sc.IsValid() {
//	    // Use the span context
//	}
func SpanContextFromContext(ctx context.Context) SpanContext {
	return SpanFromContext(ctx).SpanContext()
}

// TraceIDFromContext returns the trace ID from the current span.
// Returns an empty string if there's no valid span in the context.
// Useful for correlating logs with traces.
//
// Example:
//
//	traceID := tracing.TraceIDFromContext(ctx)
//	logger.Info("processing", slog.String("trace_id", traceID))
func TraceIDFromContext(ctx context.Context) string {
	sc := SpanContextFromContext(ctx)
	if sc == nil || !sc.IsValid() {
		return ""
	}
	return sc.TraceID()
}

// SpanIDFromContext returns the span ID from the current span.
// Returns an empty string if there's no valid span in the context.
//
// Example:
//
//	spanID := tracing.SpanIDFromContext(ctx)
//	logger.Info("processing", slog.String("span_id", spanID))
func SpanIDFromContext(ctx context.Context) string {
	sc := SpanContextFromContext(ctx)
	if sc == nil || !sc.IsValid() {
		return ""
	}
	return sc.SpanID()
}
