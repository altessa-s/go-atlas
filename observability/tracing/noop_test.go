// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"errors"
	"testing"
)

func TestNoop_ReturnsSingleton(t *testing.T) {
	a := Noop()
	b := Noop()
	if a != b {
		t.Fatal("Noop() should return the same instance")
	}
}

func TestIsNoop(t *testing.T) {
	if !IsNoop(Noop()) {
		t.Error("IsNoop(Noop()) should be true")
	}
}

func TestNoopTracer_MethodsDoNotPanic(t *testing.T) {
	tr := Noop()

	rec := tr.Recorder("test")
	ctx, span := rec.Start(t.Context(), "op")
	if ctx == nil {
		t.Error("Start should return non-nil context")
	}

	span.SetName("newname")
	span.SetStatus(StatusOK, "ok")
	span.SetAttributes(String("key", "val"))
	span.RecordError(errors.New("test error"))
	span.AddEvent("event")
	span.End()

	if span.IsRecording() {
		t.Error("noop span should not be recording")
	}

	sc := span.SpanContext()
	if sc.IsValid() {
		t.Error("noop span context should not be valid")
	}
	if sc.TraceID() != "" {
		t.Error("noop TraceID should be empty")
	}
	if sc.SpanID() != "" {
		t.Error("noop SpanID should be empty")
	}
	if sc.IsSampled() {
		t.Error("noop should not be sampled")
	}
	if sc.IsRemote() {
		t.Error("noop should not be remote")
	}

	// WithScope returns noop
	scoped := tr.WithScope("scope")
	if !IsNoop(scoped) {
		t.Error("WithScope on noop should return noop")
	}

	if err := tr.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() = %v", err)
	}
	if err := tr.ForceFlush(t.Context()); err != nil {
		t.Errorf("ForceFlush() = %v", err)
	}
}
