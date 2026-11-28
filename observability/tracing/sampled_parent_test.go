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
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

func TestWithSampledParent(t *testing.T) {
	t.Parallel()
	a := &captureAdapter{MultiAdapter: adapters.NewMultiAdapter()}
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithSampler(sampler.NewParentBased(sampler.AlwaysOn())))
	t.Cleanup(func() { require.NoError(t, tr.Shutdown(context.Background())) })
	rec := tr.Recorder("test")

	ctx, orphan := rec.Start(t.Context(), "orphan", tracing.WithSampledParent())
	require.False(t, orphan.IsRecording(), "no parent")
	require.Equal(t, t.Context(), ctx, "the context is returned unchanged")
	orphan.End()

	parentCtx, parent := rec.Start(t.Context(), "parent")
	require.True(t, parent.IsRecording())
	childCtx, child := rec.Start(parentCtx, "child", tracing.WithSampledParent(), tracing.WithSpanKind(tracing.SpanKindClient))
	require.True(t, child.IsRecording(), "a sampled parent")
	require.Equal(t, parent.SpanContext().TraceID(), child.SpanContext().TraceID())
	require.NotEqual(t, parentCtx, childCtx)
	child.End()
	parent.End()

	off := tracing.New(tracing.WithAdapter(adapters.NewMultiAdapter()), tracing.WithSampler(sampler.AlwaysOff()))
	t.Cleanup(func() { require.NoError(t, off.Shutdown(context.Background())) })
	unsampledCtx, unsampled := off.Recorder("test").Start(t.Context(), "unsampled")
	require.False(t, unsampled.SpanContext().IsSampled())
	_, child = rec.Start(unsampledCtx, "child", tracing.WithSampledParent())
	require.False(t, child.IsRecording(), "an unsampled parent")

	traceID, spanID := [16]byte{1, 2, 3}, [8]byte{4, 5, 6}
	remoteCtx := tracing.ContextWithSpanContext(t.Context(), tracing.NewRemoteSpanContext(traceID, spanID, tracing.FlagsSampled))
	_, remote := rec.Start(remoteCtx, "remote child", tracing.WithSampledParent())
	require.True(t, remote.IsRecording(), "a sampled remote parent")
	require.Equal(t, tracing.SpanContextFromContext(remoteCtx).TraceID(), remote.SpanContext().TraceID())
	remote.End()
	unsampledRemote := tracing.ContextWithSpanContext(t.Context(), tracing.NewRemoteSpanContext(traceID, spanID, 0))
	_, child = rec.Start(unsampledRemote, "child", tracing.WithSampledParent())
	require.False(t, child.IsRecording(), "an unsampled remote parent")

	_, root := rec.Start(t.Context(), "root")
	require.True(t, root.IsRecording(), "without the option a span may start a trace")
	root.End()

	require.NoError(t, tr.ForceFlush(t.Context()))
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.spans))
	for _, s := range a.spans {
		names = append(names, s.Name)
	}
	require.ElementsMatch(t, []string{"child", "parent", "remote child", "root"}, names)
}
