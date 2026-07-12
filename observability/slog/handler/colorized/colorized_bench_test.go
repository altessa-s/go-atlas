// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func benchRecord() slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	r.AddAttrs(
		slog.String("key", "value"),
		slog.Int("count", 42),
		slog.Bool("ok", true),
		slog.Duration("elapsed", 1500*time.Millisecond),
	)
	return r
}

func BenchmarkHandle(b *testing.B) {
	h := NewHandler(io.Discard, WithLevel(slog.LevelDebug))
	rec := benchRecord()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandle_WithGroupAndAttrs(b *testing.B) {
	h := NewHandler(io.Discard, WithLevel(slog.LevelDebug)).
		WithAttrs([]slog.Attr{slog.String("app", "atlas")}).
		WithGroup("req")
	rec := benchRecord()
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatTime(b *testing.B) {
	ts := time.Now()

	b.ReportAllocs()
	for b.Loop() {
		_ = formatTime(ts, time.RFC3339)
	}
}
