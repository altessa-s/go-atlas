// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"encoding/hex"
	"errors"
	"runtime"
	"sync"
	"testing"
	"weak"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

type captureAdapter struct {
	*adapters.MultiAdapter
	mu    sync.Mutex
	spans []adapters.SpanData
}

func (a *captureAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) (adapters.ExportResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.spans = append(a.spans, spans...)
	return adapters.ExportResult{Accepted: len(spans)}, nil
}

func hexID(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

func TestRecorder_ExportCarriesIDsAndScope(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	rec := tr.Recorder("svc", tracing.WithInstrumentationVersion("1.2.3"), tracing.WithSchemaURL("https://schema"))

	ctx, parent := rec.Start(t.Context(), "parent")
	_, child := rec.Start(ctx, "child", tracing.WithLinks(tracing.Link{SpanContext: parent.SpanContext()}))
	child.End()
	parent.End()

	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 2)
	got := a.spans[0]
	psc := parent.SpanContext()

	require.Equal(t, hexID(t, psc.TraceID()), got.TraceID[:])
	require.Equal(t, hexID(t, psc.SpanID()), got.ParentID[:])
	require.Len(t, got.Links, 1)
	require.Equal(t, hexID(t, psc.TraceID()), got.Links[0].TraceID[:])
	require.Equal(t, hexID(t, psc.SpanID()), got.Links[0].SpanID[:])

	for _, s := range a.spans {
		require.NotNil(t, s.InstrumentationScope)
		require.Equal(t, adapters.InstrumentationScope{Name: "svc", Version: "1.2.3", SchemaURL: "https://schema"}, *s.InstrumentationScope)
	}
}

func TestNewTreatsTypedNilDependenciesAsAbsent(t *testing.T) {
	var adapter *captureAdapter
	tr := tracing.New(tracing.WithAdapter(adapter))
	require.True(t, tracing.IsNoop(tr))

	type optionalSampler struct{ sampler.Sampler }
	var sample *optionalSampler
	tr = tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sample))
	defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()
	_, span := tr.Recorder("test").Start(t.Context(), "call")
	require.True(t, span.IsRecording())
	span.End()
}

func TestRecorderKeepsDistinctInstrumentationScopes(t *testing.T) {
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a))
	scopes := []adapters.InstrumentationScope{
		{Name: "lib", Version: "1", SchemaURL: "https://schema.example/a"},
		{Name: "lib", Version: "2", SchemaURL: "https://schema.example/a"},
		{Name: "lib", Version: "2", SchemaURL: "https://schema.example/b"},
	}
	for _, scope := range scopes {
		opts := []tracing.RecorderOption{tracing.WithInstrumentationVersion(scope.Version), tracing.WithSchemaURL(scope.SchemaURL)}
		r := tr.Recorder(scope.Name, opts...)
		require.Same(t, r, tr.Recorder(scope.Name, opts...))
		_, span := r.Start(t.Context(), "call")
		span.End()
	}
	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, len(scopes))
	for i, span := range a.spans {
		require.Equal(t, scopes[i], *span.InstrumentationScope)
	}
}

func TestRetainedExportMetadataReleasesSpanAndTracer(t *testing.T) {
	type samplingPolicy struct{ sampler.Sampler }
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	policy := &samplingPolicy{sampler.AlwaysOn()}
	policyRef := weak.Make(policy)
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(policy))
	rec := tr.Recorder("test")

	payload := new([1024]byte)
	payloadRef := weak.Make(payload)
	_, span := rec.Start(t.Context(), "request", tracing.WithAttributes(tracing.Attribute{Key: "payload", Value: payload}))
	sc := span.SpanContext()
	span.End()
	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 1)
	scope := a.spans[0].InstrumentationScope
	a.spans = nil

	runtime.GC()
	if payloadRef.Value() != nil {
		t.Error("retaining SpanContext kept the ended span's payload alive")
	}
	if policyRef.Value() != nil {
		t.Error("retaining metadata kept the tracer's sampler alive")
	}
	runtime.KeepAlive(sc)
	runtime.KeepAlive(scope)
}

func TestSpan_ZeroAndInvalidLinksFiltered(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	rec := tr.Recorder("svc")

	_, parent := rec.Start(t.Context(), "parent")
	_, noopSpan := tracing.Noop().Recorder("noop").Start(t.Context(), "noop")
	var typedNil *captureSpanContext

	_, child := rec.Start(t.Context(), "child", tracing.WithLinks(
		tracing.Link{},
		tracing.Link{SpanContext: noopSpan.SpanContext()},
		tracing.Link{SpanContext: typedNil},
		tracing.Link{SpanContext: parent.SpanContext(), Attributes: []tracing.Attribute{tracing.String("k", "v")}},
	))
	require.NotPanics(t, func() { child.End() })
	parent.End()

	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 2)
	got := a.spans[0]
	require.Len(t, got.Links, 1)
	require.Equal(t, hexID(t, parent.SpanContext().TraceID()), got.Links[0].TraceID[:])
	require.Equal(t, hexID(t, parent.SpanContext().SpanID()), got.Links[0].SpanID[:])
}

type captureSpanContext struct{ tracing.SpanContext }

type inspectingSampler struct{}

func (inspectingSampler) Description() string { return "inspecting" }

func (inspectingSampler) ShouldSample(params sampler.SamplingParameters) sampler.SamplingResult {
	var hasAttr bool
	for _, attr := range params.Attributes {
		if attr.Key == "tenant" && attr.Value == "acme" {
			hasAttr = true
		}
	}
	if !hasAttr || len(params.Links) != 1 {
		return sampler.SamplingResult{Decision: sampler.Drop}
	}
	return sampler.SamplingResult{
		Decision:   sampler.RecordAndSample,
		Attributes: []sampler.Attribute{{Key: "sampler.marker", Value: "matched"}},
	}
}

func TestSampler_ReceivesAttributesAndLinksAndMergesResultAttributes(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	refTr := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOn()))
	defer func() { require.NoError(t, refTr.Shutdown(t.Context())) }()
	_, linked := refTr.Recorder("ref").Start(t.Context(), "linked")
	defer linked.End()

	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(inspectingSampler{}))
	_, span := tr.Recorder("svc").Start(t.Context(), "op",
		tracing.WithAttributes(tracing.String("tenant", "acme")),
		tracing.WithLinks(tracing.Link{SpanContext: linked.SpanContext()}),
	)
	span.End()
	require.NoError(t, tr.Shutdown(t.Context()))

	require.Len(t, a.spans, 1)
	got := a.spans[0]
	require.Contains(t, got.Attributes, adapters.Attribute{Key: "tenant", Value: "acme"})
	require.Contains(t, got.Attributes, adapters.Attribute{Key: "sampler.marker", Value: "matched"})
}

func TestSpan_WithStackTrace(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	_, span := tr.Recorder("svc").Start(t.Context(), "op")

	errBoom := errors.New("boom")
	span.RecordError(errBoom, tracing.WithStackTrace(true))
	span.RecordError(errBoom)
	span.AddEvent("with-stack", tracing.WithStackTrace(true))
	span.AddEvent("without-stack")
	span.End()

	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 1)
	events := a.spans[0].Events
	require.Len(t, events, 4)

	findStack := func(ev adapters.SpanEvent) (string, bool) {
		for _, attr := range ev.Attributes {
			if attr.Key == "exception.stacktrace" {
				s, _ := attr.Value.(string)
				return s, true
			}
		}
		return "", false
	}

	stack0, ok0 := findStack(events[0])
	require.True(t, ok0)
	require.Contains(t, stack0, "TestSpan_WithStackTrace")

	_, ok1 := findStack(events[1])
	require.False(t, ok1)

	stack2, ok2 := findStack(events[2])
	require.True(t, ok2)
	require.Contains(t, stack2, "TestSpan_WithStackTrace")

	_, ok3 := findStack(events[3])
	require.False(t, ok3)
}

func TestSpan_SetAttributesLastWins(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.AlwaysOn()))
	_, span := tr.Recorder("svc").Start(t.Context(), "op",
		tracing.WithAttributes(tracing.String("k1", "first"), tracing.String("k2", "keep")),
	)
	span.SetAttributes(tracing.String("k1", "second"), tracing.String("k1", "third"))
	span.End()

	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 1)
	require.Equal(t, []adapters.Attribute{
		{Key: "k1", Value: "third"},
		{Key: "k2", Value: "keep"},
	}, a.spans[0].Attributes)
}

func TestSpan_LimitsCapAndReportDroppedCounts(t *testing.T) {
	t.Parallel()

	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	refTr := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOn()))
	defer func() { require.NoError(t, refTr.Shutdown(t.Context())) }()
	var links []tracing.Link
	for range 4 {
		_, l := refTr.Recorder("ref").Start(t.Context(), "linked")
		l.End()
		links = append(links, tracing.Link{SpanContext: l.SpanContext()})
	}

	tr := tracing.New(
		tracing.WithAdapter(a),
		tracing.WithSampler(sampler.AlwaysOn()),
		tracing.WithMaxAttributes(2),
		tracing.WithMaxEvents(2),
		tracing.WithMaxLinks(2),
	)
	_, span := tr.Recorder("svc").Start(t.Context(), "limited",
		tracing.WithAttributes(tracing.String("a", "1"), tracing.String("b", "2"), tracing.String("c", "3")),
		tracing.WithLinks(links...),
	)
	// Updating an existing key at capacity overwrites without dropping.
	span.SetAttributes(tracing.String("a", "updated"), tracing.String("d", "4"))
	span.AddEvent("ev1")
	span.RecordError(errors.New("err2"))
	span.AddEvent("ev3")
	span.End()

	require.NoError(t, tr.Shutdown(t.Context()))
	require.Len(t, a.spans, 1)
	got := a.spans[0]

	require.Equal(t, []adapters.Attribute{
		{Key: "a", Value: "updated"},
		{Key: "b", Value: "2"},
	}, got.Attributes)
	require.Equal(t, 2, got.DroppedAttributeCount)
	require.Len(t, got.Events, 2)
	require.Equal(t, 1, got.DroppedEventCount)
	require.Len(t, got.Links, 2)
	require.Equal(t, 2, got.DroppedLinkCount)
}
