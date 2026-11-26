// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

// discardAdapter drops exported spans so benchmarks measure the tracer alone.
type discardAdapter struct {
	*adapters.MultiAdapter
}

func (discardAdapter) ExportSpans(_ context.Context, spans []adapters.SpanData) (adapters.ExportResult, error) {
	return adapters.ExportResult{Accepted: len(spans)}, nil
}

func newBenchRecorder(b *testing.B) tracing.Recorder {
	tr := tracing.New(
		tracing.WithAdapter(discardAdapter{MultiAdapter: adapters.NewMultiAdapter()}),
		tracing.WithSampler(sampler.NewParentBased(sampler.AlwaysOn())),
	)
	b.Cleanup(func() { _ = tr.Shutdown(context.Background()) })
	return tr.Recorder("bench")
}

// BenchmarkRecorderStartEnd measures a full span lifecycle: Start, End and enqueue.
func BenchmarkRecorderStartEnd(b *testing.B) {
	rec := newBenchRecorder(b)

	b.Run("root", func(b *testing.B) {
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, span := rec.Start(ctx, "op")
			span.End()
		}
	})

	b.Run("child", func(b *testing.B) {
		ctx, parent := rec.Start(b.Context(), "parent")
		defer parent.End()
		b.ReportAllocs()
		for b.Loop() {
			_, span := rec.Start(ctx, "op")
			span.End()
		}
	})

	b.Run("child_with_link", func(b *testing.B) {
		ctx, parent := rec.Start(b.Context(), "parent")
		defer parent.End()
		link := tracing.Link{SpanContext: parent.SpanContext()}
		b.ReportAllocs()
		for b.Loop() {
			_, span := rec.Start(ctx, "op", tracing.WithLinks(link))
			span.End()
		}
	})

	b.Run("child_parallel", func(b *testing.B) {
		ctx, parent := rec.Start(b.Context(), "parent")
		defer parent.End()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, span := rec.Start(ctx, "op")
				span.End()
			}
		})
	})
}

// BenchmarkSpanSetAttributes measures attribute insertion and last-wins updates
// across small and large key sets.
func BenchmarkSpanSetAttributes(b *testing.B) {
	rec := newBenchRecorder(b)

	makeAttrs := func(n int) []tracing.Attribute {
		attrs := make([]tracing.Attribute, n)
		for i := range n {
			attrs[i] = tracing.Int(fmt.Sprintf("attr.key.%d", i), i)
		}
		return attrs
	}

	b.Run("few_keys_8", func(b *testing.B) {
		attrs := makeAttrs(8)
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, span := rec.Start(ctx, "op")
			span.SetAttributes(attrs...)
			span.SetAttributes(attrs[0], attrs[7])
			span.End()
		}
	})

	b.Run("many_keys_128", func(b *testing.B) {
		attrs := makeAttrs(128)
		ctx := b.Context()
		b.ReportAllocs()
		for b.Loop() {
			_, span := rec.Start(ctx, "op")
			span.SetAttributes(attrs...)
			span.SetAttributes(attrs[0], attrs[64], attrs[127])
			span.End()
		}
	})
}
