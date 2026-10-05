// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

func TestExtractStartPreservesRemoteParent(t *testing.T) {
	t.Parallel()
	p := propagation.NewTraceContext()
	in := propagation.MapCarrier{"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}
	ctx := p.Extract(t.Context(), in)
	remote := propagation.RemoteSpanContextFromContext(ctx)
	require.NotNil(t, remote)
	tr := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOn()))
	_, span := tr.Recorder("audit").Start(ctx, "request")
	defer span.End()
	require.Equal(t, remote.TraceID(), span.SpanContext().TraceID())
}

func TestDropPreservesPropagationContext(t *testing.T) {
	t.Parallel()
	on := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOn()))
	ctx, parent := on.Recorder("audit").Start(t.Context(), "parent")
	defer parent.End()
	require.True(t, parent.SpanContext().IsValid())
	off := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOff()))
	ctx, child := off.Recorder("audit").Start(ctx, "child")
	defer child.End()
	out := propagation.MapCarrier{}
	propagation.NewTraceContext().Inject(ctx, out)
	require.NotEmpty(t, out["traceparent"])
	require.Equal(t, parent.SpanContext().TraceID(), child.SpanContext().TraceID())
	require.False(t, child.IsRecording())
	require.False(t, child.SpanContext().IsSampled())
}

type recordOnlySampler struct{}

func (recordOnlySampler) ShouldSample(sampler.SamplingParameters) sampler.SamplingResult {
	return sampler.SamplingResult{Decision: sampler.RecordOnly}
}
func (recordOnlySampler) Description() string { return "record only" }

type countingAdapter struct {
	*adapters.MultiAdapter
	count int
}

func (a *countingAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	a.count += len(spans)
	return nil
}
func TestRecordOnlyDoesNotExport(t *testing.T) {
	t.Parallel()
	a := &countingAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(recordOnlySampler{}))
	_, span := tr.Recorder("test").Start(t.Context(), "local")
	require.True(t, span.IsRecording())
	require.True(t, span.SpanContext().IsValid())
	require.False(t, span.SpanContext().IsSampled())
	span.End()
	require.Zero(t, a.count)
}
func TestDefaultSamplerPropagatesUnsampledRemoteTrace(t *testing.T) {
	t.Parallel()
	p := propagation.NewTraceContext()
	in := propagation.MapCarrier{"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00", "tracestate": "vendor=value"}
	ctx := p.Extract(t.Context(), in)
	tr := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()))
	ctx, span := tr.Recorder("test").Start(ctx, "child")
	defer span.End()
	out := propagation.MapCarrier{}
	p.Inject(ctx, out)
	require.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-"+span.SpanContext().SpanID()+"-00", out["traceparent"])
	require.Equal(t, "vendor=value", out["tracestate"])
	require.False(t, span.IsRecording())
}
