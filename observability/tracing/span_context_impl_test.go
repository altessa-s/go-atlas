// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func TestNewSpanContextImpl_NoParent(t *testing.T) {
	sc := newSpanContextImpl(nil)
	if !sc.IsValid() {
		t.Error("new span context should be valid")
	}
	if sc.TraceID() == "" {
		t.Error("TraceID should not be empty")
	}
	if sc.SpanID() == "" {
		t.Error("SpanID should not be empty")
	}
	if len(sc.TraceID()) != 32 {
		t.Errorf("TraceID length = %d, want 32", len(sc.TraceID()))
	}
	if len(sc.SpanID()) != 16 {
		t.Errorf("SpanID length = %d, want 16", len(sc.SpanID()))
	}
	if !sc.IsSampled() {
		t.Error("new span should be sampled by default")
	}
	if sc.IsRemote() {
		t.Error("new span should not be remote")
	}
}

func TestNewSpanContextImpl_WithParent(t *testing.T) {
	parent := newSpanContextImpl(nil)
	child := newSpanContextImpl(parent)

	if child.TraceID() != parent.TraceID() {
		t.Error("child should inherit parent trace ID")
	}
	if child.SpanID() == parent.SpanID() {
		t.Error("child should have different span ID")
	}
}

func TestNewRemoteSpanContext(t *testing.T) {
	traceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	sc := NewRemoteSpanContext(traceID, spanID, FlagsSampled)

	if !sc.IsValid() {
		t.Error("should be valid")
	}
	if !sc.IsRemote() {
		t.Error("should be remote")
	}
	if !sc.IsSampled() {
		t.Error("should be sampled")
	}
}

func TestSpanContextImpl_IsValid_ZeroTraceID(t *testing.T) {
	sc := &spanContextImpl{
		spanID: [8]byte{1},
	}
	if sc.IsValid() {
		t.Error("zero traceID should be invalid")
	}
}

func TestSpanContextImpl_IsValid_ZeroSpanID(t *testing.T) {
	sc := &spanContextImpl{
		traceID: [16]byte{1},
	}
	if sc.IsValid() {
		t.Error("zero spanID should be invalid")
	}
}

func TestSpanContextImpl_IsValid_Nil(t *testing.T) {
	var sc *spanContextImpl
	if sc.IsValid() {
		t.Error("nil should be invalid")
	}
}

func TestSpanContextImpl_Bytes(t *testing.T) {
	sc := newSpanContextImpl(nil)
	traceBytes := sc.TraceIDBytes()
	spanBytes := sc.SpanIDBytes()
	if traceBytes == [16]byte{} {
		t.Error("trace ID bytes should not be zero")
	}
	if spanBytes == [8]byte{} {
		t.Error("span ID bytes should not be zero")
	}
}

func TestNewSpanContextImpl_UniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		sc := newSpanContextImpl(nil)
		id := sc.TraceID() + sc.SpanID()
		if seen[id] {
			t.Fatal("generated duplicate ID")
		}
		seen[id] = true
	}
}
