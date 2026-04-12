// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestSpanFromContext_Empty(t *testing.T) {
	span := tracing.SpanFromContext(t.Context())
	require.NotNil(t, span, "SpanFromContext should return noop span, not nil")
	require.False(t, span.IsRecording(), "noop span should not be recording")
}

func TestSpanFromContext_NilContext(t *testing.T) {
	span := tracing.SpanFromContext(nil)
	require.NotNil(t, span, "SpanFromContext(nil) should return noop span")
}

func TestContextWithSpan_RoundTrip(t *testing.T) {
	tracer := tracing.Noop()
	rec := tracer.Recorder("test")
	ctx, span := rec.Start(t.Context(), "op")

	ctx = tracing.ContextWithSpan(ctx, span)
	got := tracing.SpanFromContext(ctx)
	require.NotNil(t, got, "SpanFromContext should return stored span")
}

func TestContextWithSpan_NilContext(t *testing.T) {
	tracer := tracing.Noop()
	rec := tracer.Recorder("test")
	_, span := rec.Start(t.Context(), "op")

	ctx := tracing.ContextWithSpan(nil, span)
	got := tracing.SpanFromContext(ctx)
	require.NotNil(t, got, "SpanFromContext should return stored span")
}

func TestSpanContextFromContext_Empty(t *testing.T) {
	sc := tracing.SpanContextFromContext(t.Context())
	require.NotNil(t, sc, "SpanContextFromContext should not return nil")
	require.False(t, sc.IsValid(), "empty context should return invalid SpanContext")
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	id := tracing.TraceIDFromContext(t.Context())
	require.Empty(t, id, "TraceIDFromContext(empty) should be empty")
}

func TestSpanIDFromContext_Empty(t *testing.T) {
	id := tracing.SpanIDFromContext(t.Context())
	require.Empty(t, id, "SpanIDFromContext(empty) should be empty")
}

func TestNoopSpan_AllMethods(t *testing.T) {
	tracer := tracing.Noop()
	rec := tracer.Recorder("test")
	ctx, span := rec.Start(t.Context(), "op")
	_ = ctx

	// All methods should not panic
	span.End()
	span.SetName("renamed")
	span.SetStatus(tracing.StatusOK, "fine")
	span.SetAttributes(tracing.String("k", "v"))
	span.RecordError(nil)
	span.AddEvent("event")

	sc := span.SpanContext()
	_ = sc.TraceID()
	_ = sc.SpanID()
	_ = sc.TraceFlags()
	_ = sc.IsValid()
	_ = sc.IsRemote()
	_ = sc.IsSampled()
}

func TestNoopTracer_WithScope(t *testing.T) {
	tracer := tracing.Noop()
	scoped := tracer.WithScope("sub")
	require.True(t, tracing.IsNoop(scoped), "WithScope on noop should return noop")
}

func TestNoopTracer_Shutdown(t *testing.T) {
	require.NoError(t, tracing.Noop().Shutdown(t.Context()))
}

func TestNoopTracer_ForceFlush(t *testing.T) {
	require.NoError(t, tracing.Noop().ForceFlush(t.Context()))
}
