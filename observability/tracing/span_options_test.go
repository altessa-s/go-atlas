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

func TestLinksPool(t *testing.T) {
	links := GetLinks()
	require.NotNil(t, links, "GetLinks returned nil")
	PutLinks(links)

	links2 := GetLinksWithCapacity(8)
	require.NotNil(t, links2, "GetLinksWithCapacity returned nil")
	PutLinks(links2)
}
