// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoop_ReturnsSingleton(t *testing.T) {
	a := Noop()
	b := Noop()
	require.Same(t, a, b, "Noop() should return the same instance")
}

func TestIsNoop(t *testing.T) {
	require.True(t, IsNoop(Noop()), "IsNoop(Noop()) should be true")
}

func TestNoopTracer_MethodsDoNotPanic(t *testing.T) {
	tr := Noop()

	rec := tr.Recorder("test")
	ctx, span := rec.Start(t.Context(), "op")
	require.NotNil(t, ctx, "Start should return non-nil context")

	span.SetName("newname")
	span.SetStatus(StatusOK, "ok")
	span.SetAttributes(String("key", "val"))
	span.RecordError(errors.New("test error"))
	span.AddEvent("event")
	span.End()

	require.False(t, span.IsRecording(), "noop span should not be recording")

	sc := span.SpanContext()
	require.False(t, sc.IsValid(), "noop span context should not be valid")
	require.Empty(t, sc.TraceID(), "noop TraceID should be empty")
	require.Empty(t, sc.SpanID(), "noop SpanID should be empty")
	require.False(t, sc.IsSampled(), "noop should not be sampled")
	require.False(t, sc.IsRemote(), "noop should not be remote")

	// WithScope returns noop
	scoped := tr.WithScope("scope")
	require.True(t, IsNoop(scoped), "WithScope on noop should return noop")

	require.NoError(t, tr.Shutdown(t.Context()))
	require.NoError(t, tr.ForceFlush(t.Context()))
}
