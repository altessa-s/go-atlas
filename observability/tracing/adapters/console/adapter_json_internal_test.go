// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package console

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

// These white-box tests build the adapter directly to reach the JSON writer
// (selected by prettyPrint=false) and its write-error paths.

var errWrite = errors.New("write failed")

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after <= 0 {
		return 0, errWrite
	}
	w.after--
	return len(p), nil
}

func fullSpan() adapters.SpanData {
	start := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return adapters.SpanData{
		TraceID:    adapters.TraceID{0xab},
		SpanID:     adapters.SpanID{0xcd},
		ParentID:   adapters.SpanID{0xef},
		Name:       "op",
		Kind:       adapters.SpanKindClient,
		StartTime:  start,
		EndTime:    start.Add(2 * time.Second),
		Attributes: []adapters.Attribute{{Key: "k", Value: "v"}},
		Events: []adapters.SpanEvent{
			{Name: "ev", Timestamp: start, Attributes: []adapters.Attribute{{Key: "ek", Value: 1}}},
			{Name: "bare", Timestamp: start},
		},
		Links: []adapters.SpanLink{
			{TraceID: adapters.TraceID{0x11}, SpanID: adapters.SpanID{0x22}, Attributes: []adapters.Attribute{{Key: "lk", Value: true}}},
			{TraceID: adapters.TraceID{0x33}, SpanID: adapters.SpanID{0x44}},
		},
		Status:     adapters.StatusError,
		StatusDesc: "boom",
	}
}

func TestWriteJSON_FullSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := New(WithWriter(&buf), WithPrettyPrint(false))
	span := fullSpan()
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{span}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, span.TraceID.String(), got["trace_id"])
	require.Equal(t, span.SpanID.String(), got["span_id"])
	require.Equal(t, span.ParentID.String(), got["parent_id"])
	require.Equal(t, "op", got["name"])
	require.Equal(t, "client", got["kind"])
	require.Equal(t, "2026-03-04T05:06:07Z", got["start_time"])
	require.Equal(t, "2026-03-04T05:06:09Z", got["end_time"])
	require.Equal(t, "2s", got["duration"])
	require.Equal(t, "Error", got["status"])
	require.Equal(t, "boom", got["status_description"])
	require.Equal(t, map[string]any{"k": "v"}, got["attributes"])

	events := got["events"].([]any)
	require.Len(t, events, 2)
	require.Equal(t, map[string]any{"ek": float64(1)}, events[0].(map[string]any)["attributes"])
	require.NotContains(t, events[1].(map[string]any), "attributes")

	links := got["links"].([]any)
	require.Len(t, links, 2)
	require.Equal(t, adapters.TraceID{0x11}.String(), links[0].(map[string]any)["trace_id"])
	require.Equal(t, map[string]any{"lk": true}, links[0].(map[string]any)["attributes"])
	require.NotContains(t, links[1].(map[string]any), "attributes")
}

func TestWriteJSON_MinimalSpanOmitsOptionalFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := New(WithWriter(&buf), WithPrettyPrint(false))
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{{Name: "min"}}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	for _, k := range []string{"parent_id", "attributes", "events", "links", "status_description"} {
		require.NotContains(t, got, k)
	}
	require.Equal(t, "unspecified", got["kind"])
	require.Equal(t, "Unset", got["status"])
}

func TestWriteJSON_MarshalError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := New(WithWriter(&buf), WithPrettyPrint(false))
	span := adapters.SpanData{Attributes: []adapters.Attribute{{Key: "ch", Value: make(chan int)}}}
	require.Error(t, a.ExportSpans(t.Context(), []adapters.SpanData{span}))
	require.Zero(t, buf.Len())
}

func TestWriteJSON_WriterError(t *testing.T) {
	t.Parallel()

	a := New(WithWriter(&failingWriter{}), WithPrettyPrint(false))
	require.ErrorIs(t, a.ExportSpans(t.Context(), []adapters.SpanData{{Name: "x"}}), errWrite)
}

func TestWritePretty_WithoutTimestampsIncludesEvents(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := New(WithWriter(&buf), WithTimestamps(false))
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{fullSpan()}))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 4)
	require.Equal(t, "[SPAN] op (ab00000000000000/cd00000000000000) 2s Error", lines[0])
	require.Equal(t, "       k=v", lines[1])
	require.Equal(t, "  [EVENT] ev at 05:06:07.000", lines[2])
	require.Equal(t, "  [EVENT] bare at 05:06:07.000", lines[3])
}

func TestWritePretty_WriterErrors(t *testing.T) {
	t.Parallel()

	// after=0 fails the header, 1 fails the attribute line, 2 fails the first event line.
	for after := range 3 {
		a := New(WithWriter(&failingWriter{after: after}), WithTimestamps(false))
		require.ErrorIs(t, a.ExportSpans(t.Context(), []adapters.SpanData{fullSpan()}), errWrite, "after=%d", after)
	}
}

func TestNewOptions_AppliesOptions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	o := newOptions(WithWriter(&buf))
	require.Same(t, &buf, o.writer)
	require.True(t, o.prettyPrint)
	require.True(t, o.timestamps)
}
