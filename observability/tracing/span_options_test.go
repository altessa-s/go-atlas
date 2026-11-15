// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyRecorderOptions(t *testing.T) {
	cfg := ApplyRecorderOptions(
		WithInstrumentationVersion("1.0.0"),
		WithSchemaURL("https://schema.example.com"),
	)
	require.Equal(t, "1.0.0", cfg.InstrumentationVersion())
	require.Equal(t, "https://schema.example.com", cfg.SchemaURL())
}

func TestApplySpanStartOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplySpanStartOptions(
		WithSpanKind(SpanKindServer),
		WithAttributes(String("key", "val")),
		WithStartTimestamp(now),
	)
	require.Equal(t, SpanKindServer, cfg.Kind())
	require.Len(t, cfg.Attributes(), 1)
	require.True(t, cfg.Timestamp().Equal(now))
}

func TestApplySpanEndOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplySpanEndOptions(WithEndTimestamp(now))
	require.True(t, cfg.Timestamp().Equal(now))
}

func TestApplyEventOptions(t *testing.T) {
	now := time.Now()
	cfg := ApplyEventOptions(
		WithEventTimestamp(now),
		WithEventAttributes(String("k", "v")),
		WithStackTrace(true),
	)
	require.True(t, cfg.Timestamp().Equal(now))
	require.Len(t, cfg.Attributes(), 1)
	require.True(t, cfg.StackTrace(), "StackTrace should be true")
}

func TestApplySpanStartOptions_Links(t *testing.T) {
	sc := newSpanContextImpl(nil)
	link := Link{SpanContext: sc}
	cfg := ApplySpanStartOptions(WithLinks(link))
	require.Len(t, cfg.Links(), 1)
}

// Without options every resolved config is the zero value: unspecified kind,
// zero timestamps (the recorder substitutes the current time), nil — not
// empty — attribute and link slices, and no stack trace.
func TestApplyOptions_ZeroValues(t *testing.T) {
	t.Parallel()

	rec := ApplyRecorderOptions()
	require.Empty(t, rec.InstrumentationVersion())
	require.Empty(t, rec.SchemaURL())

	start := ApplySpanStartOptions()
	require.Equal(t, SpanKindUnspecified, start.Kind())
	require.True(t, start.Timestamp().IsZero())
	require.Nil(t, start.Attributes())
	require.Nil(t, start.Links())

	require.True(t, ApplySpanEndOptions().Timestamp().IsZero())

	event := ApplyEventOptions()
	require.True(t, event.Timestamp().IsZero())
	require.Nil(t, event.Attributes())
	require.False(t, event.StackTrace())
}

// An option given no values leaves the slice nil instead of allocating an
// empty one.
func TestApplyOptions_EmptyVariadicKeepsNil(t *testing.T) {
	t.Parallel()

	start := ApplySpanStartOptions(WithAttributes(), WithLinks())
	require.Nil(t, start.Attributes())
	require.Nil(t, start.Links())

	require.Nil(t, ApplyEventOptions(WithEventAttributes()).Attributes())
}

// Slice options accumulate in call order and keep duplicates; scalar options
// are last-wins, and a zero value overrides an earlier non-zero one.
func TestApplyOptions_AccumulateAndOverride(t *testing.T) {
	t.Parallel()

	a, b := String("k", "a"), String("k", "b")
	link := Link{SpanContext: newSpanContextImpl(nil)}

	start := ApplySpanStartOptions(
		WithSpanKind(SpanKindServer),
		WithAttributes(a, a),
		WithLinks(link),
		WithAttributes(b),
		WithLinks(link),
		WithStartTimestamp(time.Now()),
		WithSpanKind(SpanKindUnspecified),
		WithStartTimestamp(time.Time{}),
	)
	require.Equal(t, []Attribute{a, a, b}, start.Attributes())
	require.Len(t, start.Links(), 2)
	require.Equal(t, SpanKindUnspecified, start.Kind())
	require.True(t, start.Timestamp().IsZero())

	event := ApplyEventOptions(
		WithEventAttributes(a, a),
		WithStackTrace(true),
		WithEventAttributes(b),
		WithStackTrace(false),
	)
	require.Equal(t, []Attribute{a, a, b}, event.Attributes())
	require.False(t, event.StackTrace())

	rec := ApplyRecorderOptions(
		WithInstrumentationVersion(" 1.0.0 "),
		WithSchemaURL("https://schema.example.com"),
		WithSchemaURL(""),
	)
	require.Equal(t, " 1.0.0 ", rec.InstrumentationVersion(), "the version is stored verbatim")
	require.Empty(t, rec.SchemaURL(), "an empty string overrides an earlier value")
}

// The resolved config copies the caller's slice, so a later write to that
// slice does not reach it, and the same options can be applied again.
func TestApplySpanStartOptions_DoesNotAliasInput(t *testing.T) {
	t.Parallel()

	in := []Attribute{String("k", "a")}
	opts := []SpanStartOption{WithAttributes(in...), WithAttributes(String("k", "b"))}

	cfg := ApplySpanStartOptions(opts...)
	in[0] = String("k", "changed")
	require.Equal(t, []Attribute{String("k", "a"), String("k", "b")}, cfg.Attributes())
	require.Len(t, ApplySpanStartOptions(opts...).Attributes(), 2, "options are reusable")
}

func TestLinksPool(t *testing.T) {
	links := GetLinks()
	require.NotNil(t, links, "GetLinks returned nil")
	PutLinks(links)

	links2 := GetLinksWithCapacity(8)
	require.NotNil(t, links2, "GetLinksWithCapacity returned nil")
	PutLinks(links2)
}
