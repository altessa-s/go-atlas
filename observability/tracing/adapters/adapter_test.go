// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
)

func TestTraceIDAndSpanID(t *testing.T) {
	t.Parallel()

	var zeroTrace adapters.TraceID
	var zeroSpan adapters.SpanID
	require.False(t, zeroTrace.IsValid())
	require.False(t, zeroSpan.IsValid())
	require.Equal(t, "00000000000000000000000000000000", zeroTrace.String())
	require.Equal(t, "0000000000000000", zeroSpan.String())

	tid := adapters.TraceID{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}
	sid := adapters.SpanID{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0xff}
	require.True(t, tid.IsValid())
	require.True(t, sid.IsValid())
	require.Equal(t, "0123456789abcdeffedcba9876543210", tid.String())
	require.Equal(t, "deadbeef000102ff", sid.String())
}

func TestSpanKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind adapters.SpanKind
		want string
	}{
		{adapters.SpanKindUnspecified, "unspecified"},
		{adapters.SpanKindInternal, "internal"},
		{adapters.SpanKindServer, "server"},
		{adapters.SpanKindClient, "client"},
		{adapters.SpanKindProducer, "producer"},
		{adapters.SpanKindConsumer, "consumer"},
		{adapters.SpanKind(99), "unspecified"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.kind.String())
		})
	}
}

func TestStatusCodeString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Unset", adapters.StatusUnset.String())
	require.Equal(t, "OK", adapters.StatusOK.String())
	require.Equal(t, "Error", adapters.StatusError.String())
	require.Equal(t, "Unset", adapters.StatusCode(42).String())
}

func TestSpanDataAccessors(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	span := adapters.SpanData{
		StartTime:  start,
		EndTime:    start.Add(1500 * time.Millisecond),
		Attributes: []adapters.Attribute{{Key: "a", Value: 1}, {Key: "b", Value: "x"}},
		Events:     []adapters.SpanEvent{{Name: "e1"}},
		Links:      []adapters.SpanLink{{TraceState: "s1"}, {TraceState: "s2"}},
	}

	require.Equal(t, 1500*time.Millisecond, span.Duration())
	require.Equal(t, span.Attributes, slices.Collect(span.AttributesIter()))
	require.Equal(t, span.Events, slices.Collect(span.EventsIter()))
	require.Equal(t, span.Links, slices.Collect(span.LinksIter()))
}

type recordingAdapter struct {
	name                             string
	exportErr, shutdownErr, flushErr error
	rejected                         int

	exported  int
	shutdowns int
	flushes   int
}

func (r *recordingAdapter) Name() string {
	if r.name != "" {
		return r.name
	}
	return "recording"
}

func (r *recordingAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) (adapters.ExportResult, error) {
	r.exported += len(spans)
	if r.exportErr != nil {
		return adapters.ExportResult{}, r.exportErr
	}
	rej := min(max(r.rejected, 0), len(spans))
	return adapters.ExportResult{
		Accepted: len(spans) - rej,
		Rejected: rej,
	}, nil
}

func (r *recordingAdapter) Shutdown(context.Context) error {
	r.shutdowns++
	return r.shutdownErr
}

func (r *recordingAdapter) ForceFlush(context.Context) error {
	r.flushes++
	return r.flushErr
}

func TestMultiAdapterBroadcasts(t *testing.T) {
	t.Parallel()

	a, b := &recordingAdapter{}, &recordingAdapter{}
	m := adapters.NewMultiAdapter(a, b)
	require.Equal(t, "multi", m.Name())

	spans := []adapters.SpanData{{Name: "one"}, {Name: "two"}}
	res, err := m.ExportSpans(t.Context(), spans)
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{Accepted: 2, Rejected: 0}, res)
	require.NoError(t, m.ForceFlush(t.Context()))
	require.NoError(t, m.Shutdown(t.Context()))

	for _, r := range []*recordingAdapter{a, b} {
		require.Equal(t, 2, r.exported)
		require.Equal(t, 1, r.flushes)
		require.Equal(t, 1, r.shutdowns)
	}
}

func TestMultiAdapterPropagatesErrors(t *testing.T) {
	t.Parallel()

	errExport := errors.New("export")
	errShutdown := errors.New("shutdown")
	errFlush := errors.New("flush")
	failing := &recordingAdapter{exportErr: errExport, shutdownErr: errShutdown, flushErr: errFlush}
	m := adapters.NewMultiAdapter(&recordingAdapter{}, failing)

	res, err := m.ExportSpans(t.Context(), []adapters.SpanData{{}})
	require.ErrorIs(t, err, errExport)
	require.Equal(t, adapters.ExportResult{Accepted: 0, Rejected: 0}, res)
	require.ErrorIs(t, m.ForceFlush(t.Context()), errFlush)
	require.ErrorIs(t, m.Shutdown(t.Context()), errShutdown)
}

func TestMultiAdapterContinuesPastFailuresAndJoinsErrors(t *testing.T) {
	t.Parallel()

	errExport1, errExport2 := errors.New("export 1"), errors.New("export 2")
	errShutdown1, errShutdown2 := errors.New("shutdown 1"), errors.New("shutdown 2")
	errFlush1, errFlush2 := errors.New("flush 1"), errors.New("flush 2")
	first := &recordingAdapter{exportErr: errExport1, shutdownErr: errShutdown1, flushErr: errFlush1}
	second := &recordingAdapter{exportErr: errExport2, shutdownErr: errShutdown2, flushErr: errFlush2}
	healthy := &recordingAdapter{}
	m := adapters.NewMultiAdapter(first, second, healthy)

	_, err := m.ExportSpans(t.Context(), []adapters.SpanData{{}})
	require.ErrorIs(t, err, errExport1)
	require.ErrorIs(t, err, errExport2)

	err = m.ForceFlush(t.Context())
	require.ErrorIs(t, err, errFlush1)
	require.ErrorIs(t, err, errFlush2)

	err = m.Shutdown(t.Context())
	require.ErrorIs(t, err, errShutdown1)
	require.ErrorIs(t, err, errShutdown2)

	// Every adapter after a failing one must still receive each call.
	for _, r := range []*recordingAdapter{first, second, healthy} {
		require.Equal(t, 1, r.exported)
		require.Equal(t, 1, r.flushes)
		require.Equal(t, 1, r.shutdowns)
	}
}

func TestMultiAdapterPartialRejection(t *testing.T) {
	t.Parallel()

	first := &recordingAdapter{name: "first", rejected: 1}
	second := &recordingAdapter{name: "second", rejected: 2}
	m := adapters.NewMultiAdapter(first, second)

	spans := []adapters.SpanData{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	res, err := m.ExportSpans(t.Context(), spans)
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{Accepted: 1, Rejected: 2}, res)
	require.Len(t, m.Adapters(), 2)
}

func TestMultiAdapterEmpty(t *testing.T) {
	t.Parallel()

	m := adapters.NewMultiAdapter()
	res, err := m.ExportSpans(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{}, res)
	require.NoError(t, m.ForceFlush(t.Context()))
	require.NoError(t, m.Shutdown(t.Context()))
}

type oneSpanWriter struct {
	wroteLine bool
	err       error
}

func (w *oneSpanWriter) Write(p []byte) (int, error) {
	if w.wroteLine {
		return 0, w.err
	}
	if bytes.IndexByte(p, '\n') >= 0 {
		w.wroteLine = true
	}
	return len(p), nil
}

func TestMultiAdapterPreservesPartialAcceptanceOnError(t *testing.T) {
	t.Parallel()

	errWrite := errors.New("write failed on second span")
	partial := console.New(
		console.WithWriter(&oneSpanWriter{err: errWrite}),
		console.WithPrettyPrint(false),
	)
	healthy := &recordingAdapter{name: "healthy"}
	m := adapters.NewMultiAdapter(healthy, partial)

	spans := []adapters.SpanData{{Name: "first"}, {Name: "second"}}
	res, err := m.ExportSpans(t.Context(), spans)
	require.ErrorIs(t, err, errWrite)
	require.Equal(t, adapters.ExportResult{Accepted: 1, Rejected: 0}, res)
	require.Equal(t, 2, healthy.exported)
}
