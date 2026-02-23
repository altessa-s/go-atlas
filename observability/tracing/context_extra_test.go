// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"
)

func TestSpanFromContext_Empty(t *testing.T) {
	span := tracing.SpanFromContext(t.Context())
	if span == nil {
		t.Fatal("SpanFromContext should return noop span, not nil")
	}
	if span.IsRecording() {
		t.Error("noop span should not be recording")
	}
}

func TestSpanFromContext_NilContext(t *testing.T) {
	span := tracing.SpanFromContext(nil)
	if span == nil {
		t.Fatal("SpanFromContext(nil) should return noop span")
	}
}

func TestContextWithSpan_RoundTrip(t *testing.T) {
	tracer := tracing.Noop()
	rec := tracer.Recorder("test")
	ctx, span := rec.Start(t.Context(), "op")

	ctx = tracing.ContextWithSpan(ctx, span)
	got := tracing.SpanFromContext(ctx)

	if got == nil {
		t.Fatal("SpanFromContext should return stored span")
	}
}

func TestContextWithSpan_NilContext(t *testing.T) {
	tracer := tracing.Noop()
	rec := tracer.Recorder("test")
	_, span := rec.Start(t.Context(), "op")

	ctx := tracing.ContextWithSpan(nil, span)
	got := tracing.SpanFromContext(ctx)
	if got == nil {
		t.Fatal("SpanFromContext should return stored span")
	}
}

func TestSpanContextFromContext_Empty(t *testing.T) {
	sc := tracing.SpanContextFromContext(t.Context())
	if sc == nil {
		t.Fatal("SpanContextFromContext should not return nil")
	}
	if sc.IsValid() {
		t.Error("empty context should return invalid SpanContext")
	}
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	id := tracing.TraceIDFromContext(t.Context())
	if id != "" {
		t.Errorf("TraceIDFromContext(empty) = %q, want empty", id)
	}
}

func TestSpanIDFromContext_Empty(t *testing.T) {
	id := tracing.SpanIDFromContext(t.Context())
	if id != "" {
		t.Errorf("SpanIDFromContext(empty) = %q, want empty", id)
	}
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
	if !tracing.IsNoop(scoped) {
		t.Error("WithScope on noop should return noop")
	}
}

func TestNoopTracer_Shutdown(t *testing.T) {
	if err := tracing.Noop().Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNoopTracer_ForceFlush(t *testing.T) {
	if err := tracing.Noop().ForceFlush(t.Context()); err != nil {
		t.Errorf("ForceFlush() error = %v", err)
	}
}
