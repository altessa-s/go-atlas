// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSpanContextImpl_NoParent(t *testing.T) {
	sc := newSpanContextImpl(nil)
	require.True(t, sc.IsValid(), "new span context should be valid")
	require.NotEmpty(t, sc.TraceID(), "TraceID should not be empty")
	require.NotEmpty(t, sc.SpanID(), "SpanID should not be empty")
	require.Len(t, sc.TraceID(), 32)
	require.Len(t, sc.SpanID(), 16)
	require.True(t, sc.IsSampled(), "new span should be sampled by default")
	require.False(t, sc.IsRemote(), "new span should not be remote")
}

func TestNewSpanContextImpl_WithParent(t *testing.T) {
	parent := newSpanContextImpl(nil)
	child := newSpanContextImpl(parent)

	require.Equal(t, parent.TraceID(), child.TraceID(), "child should inherit parent trace ID")
	require.NotEqual(t, parent.SpanID(), child.SpanID(), "child should have different span ID")
}

func TestNewRemoteSpanContext(t *testing.T) {
	traceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	sc := NewRemoteSpanContext(traceID, spanID, FlagsSampled)

	require.True(t, sc.IsValid(), "should be valid")
	require.True(t, sc.IsRemote(), "should be remote")
	require.True(t, sc.IsSampled(), "should be sampled")
}

func TestSpanContextImpl_IsValid_ZeroTraceID(t *testing.T) {
	sc := &spanContextImpl{
		spanID: [8]byte{1},
	}
	require.False(t, sc.IsValid(), "zero traceID should be invalid")
}

func TestSpanContextImpl_IsValid_ZeroSpanID(t *testing.T) {
	sc := &spanContextImpl{
		traceID: [16]byte{1},
	}
	require.False(t, sc.IsValid(), "zero spanID should be invalid")
}

func TestSpanContextImpl_IsValid_Nil(t *testing.T) {
	var sc *spanContextImpl
	require.False(t, sc.IsValid(), "nil should be invalid")
}

func TestSpanContextImpl_Bytes(t *testing.T) {
	sc := newSpanContextImpl(nil)
	traceBytes := sc.TraceIDBytes()
	spanBytes := sc.SpanIDBytes()
	require.NotEqual(t, [16]byte{}, traceBytes, "trace ID bytes should not be zero")
	require.NotEqual(t, [8]byte{}, spanBytes, "span ID bytes should not be zero")
}

func TestNewSpanContextImpl_UniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		sc := newSpanContextImpl(nil)
		id := sc.TraceID() + sc.SpanID()
		require.False(t, seen[id], "generated duplicate ID")
		seen[id] = true
	}
}
