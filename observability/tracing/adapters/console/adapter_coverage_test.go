// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
)

func testSpan() adapters.SpanData {
	start := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	return adapters.SpanData{
		TraceID:    adapters.TraceID{0x01},
		SpanID:     adapters.SpanID{0x02},
		Name:       "req-handler",
		Kind:       adapters.SpanKindServer,
		StartTime:  start,
		EndTime:    start.Add(5 * time.Millisecond),
		Attributes: []adapters.Attribute{{Key: "http.status_code", Value: 200}},
		Status:     adapters.StatusOK,
	}
}

func TestNew_DefaultsToStdoutPrettyWithTimestamps(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := console.New(console.WithWriter(&buf))

	require.Equal(t, "console", a.Name())

	err := a.ExportSpans(t.Context(), []adapters.SpanData{testSpan()})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "[SPAN] req-handler")
	require.Regexp(t, `^\d{2}:\d{2}:\d{2}\.\d{3} `, out, "timestamps default to enabled")
}

func TestNew_NilOptionsFallBackToOSStdout(t *testing.T) {
	t.Parallel()

	// New() with no WithWriter must not panic and must pick a non-nil
	// default writer (os.Stdout) rather than leaving it nil.
	a := console.New()
	require.NotNil(t, a)
	require.Equal(t, "console", a.Name())
}

func TestExportSpans_MultipleSpansWriteSeparateLines(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := console.New(console.WithWriter(&buf))
	span := testSpan()

	err := a.ExportSpans(t.Context(), []adapters.SpanData{span, span})
	require.NoError(t, err)

	// Each span emits exactly one "[SPAN]" header line.
	require.Equal(t, 2, strings.Count(buf.String(), "[SPAN]"))
}

func TestExportSpans_EmptyBatchProducesNoOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := console.New(console.WithWriter(&buf))

	err := a.ExportSpans(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, buf.String())
}

func TestNew_WithPrettyPrintAndWithTimestamps_TrueMatchesDefaults(t *testing.T) {
	t.Parallel()

	var withOpts, withoutOpts bytes.Buffer
	span := testSpan()

	a1 := console.New(console.WithWriter(&withOpts), console.WithPrettyPrint(true), console.WithTimestamps(true))
	require.NoError(t, a1.ExportSpans(t.Context(), []adapters.SpanData{span}))

	a2 := console.New(console.WithWriter(&withoutOpts))
	require.NoError(t, a2.ExportSpans(t.Context(), []adapters.SpanData{span}))

	// Strip each line's timestamp prefix (it is wall-clock based and
	// legitimately differs between the two calls) before comparing.
	timestampPrefix := regexp.MustCompile(`(?m)^\d{2}:\d{2}:\d{2}\.\d{3} `)
	strip := func(s string) string { return timestampPrefix.ReplaceAllString(s, "") }
	require.Equal(t, strip(withoutOpts.String()), strip(withOpts.String()))
}

func TestNew_WithPrettyPrintFalse_EmitsJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := console.New(console.WithWriter(&buf), console.WithPrettyPrint(false))
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{testSpan()}))

	line := strings.TrimSpace(buf.String())
	require.NotContains(t, line, "[SPAN]")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &decoded), line)
	require.Equal(t, "req-handler", decoded["name"])
}

func TestNew_WithTimestampsFalse_OmitsTimestampPrefix(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := console.New(console.WithWriter(&buf), console.WithTimestamps(false))
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{testSpan()}))

	out := buf.String()
	require.Contains(t, out, "[SPAN] req-handler")
	require.NotRegexp(t, `(?m)^\d{2}:\d{2}:\d{2}\.\d{3} `, out)
}

func TestShutdownAndForceFlush_AreNoops(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	a := console.New(console.WithWriter(&out))
	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, a.ForceFlush(t.Context()))
	require.Zero(t, out.Len(), "shutdown and flush write nothing")
}

func TestExportSpans_CancelWhileAnotherExportWrites(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	a := console.New(console.WithWriter(writer), console.WithPrettyPrint(false))
	spans := []adapters.SpanData{testSpan()}
	first := make(chan error, 1)
	go func() { first <- a.ExportSpans(t.Context(), spans) }()
	// Read only the first byte so the writer still owns the adapter.
	var firstByte [1]byte
	_, err := io.ReadFull(reader, firstByte[:])
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	second := make(chan error, 1)
	go func() { second <- a.ExportSpans(ctx, spans) }()
	cancel()
	select {
	case err := <-second:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Error("canceled export still waiting for the writer")
	}
	require.NoError(t, reader.Close())
	require.ErrorIs(t, <-first, io.ErrClosedPipe)
}
