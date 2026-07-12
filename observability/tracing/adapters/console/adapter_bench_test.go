// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package console_test

import (
	"io"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
)

// benchSpan returns a representative span with a small attribute set.
func benchSpan() adapters.SpanData {
	end := time.Now()
	return adapters.SpanData{
		TraceID:   adapters.TraceID{0x01, 0x02, 0x03, 0x04},
		SpanID:    adapters.SpanID{0x0a, 0x0b},
		Name:      "benchmark-span",
		Kind:      adapters.SpanKindServer,
		StartTime: end.Add(-time.Millisecond),
		EndTime:   end,
		Attributes: []adapters.Attribute{
			{Key: "http.method", Value: "GET"},
			{Key: "http.status_code", Value: 200},
		},
		Status: adapters.StatusOK,
	}
}

func BenchmarkExportSpans_JSON(b *testing.B) {
	a := console.New(console.WithWriter(io.Discard))
	spans := []adapters.SpanData{benchSpan()}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := a.ExportSpans(ctx, spans); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExportSpans_Pretty(b *testing.B) {
	a := console.New(console.WithWriter(io.Discard), console.WithPrettyPrint())
	spans := []adapters.SpanData{benchSpan()}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := a.ExportSpans(ctx, spans); err != nil {
			b.Fatal(err)
		}
	}
}
