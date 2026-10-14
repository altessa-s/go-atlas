// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

// discardAdapter drops exported spans so benchmarks measure the tracer alone.
type discardAdapter struct {
	*adapters.MultiAdapter
}

func (discardAdapter) ExportSpans(context.Context, []adapters.SpanData) error { return nil }

func newBenchRecorder() tracing.Recorder {
	tr := tracing.New(
		tracing.WithAdapter(discardAdapter{MultiAdapter: adapters.NewMultiAdapter()}),
		tracing.WithSampler(sampler.NewParentBased(sampler.AlwaysOn())),
	)
	return tr.Recorder("bench")
}

// BenchmarkRecorderStartEnd measures a full span lifecycle: Start, End and
// export through the adapter.
func BenchmarkRecorderStartEnd(b *testing.B) {
	rec := newBenchRecorder()

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
