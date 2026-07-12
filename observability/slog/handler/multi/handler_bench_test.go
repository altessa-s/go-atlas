// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package multi

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// benchRecord returns a representative record with a small attribute set.
func benchRecord() slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	r.AddAttrs(
		slog.String("key", "value"),
		slog.Int("count", 42),
	)
	return r
}

func BenchmarkHandle(b *testing.B) {
	h := NewHandler(
		slog.NewTextHandler(io.Discard, nil),
		slog.NewTextHandler(io.Discard, nil),
	)
	rec := benchRecord()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandle_Concurrent(b *testing.B) {
	h := NewConcurrentHandler(
		slog.NewTextHandler(io.Discard, nil),
		slog.NewTextHandler(io.Discard, nil),
	)
	rec := benchRecord()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}
