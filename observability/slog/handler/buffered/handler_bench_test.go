// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package buffered

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// BenchmarkHandle measures the steady-state buffered path: records below the
// bypass level are enqueued while the background worker drains them into a
// discard-backed inner handler.
func BenchmarkHandle(b *testing.B) {
	h := NewHandler(slog.NewTextHandler(io.Discard, nil))
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	rec.AddAttrs(
		slog.String("key", "value"),
		slog.Int("count", 42),
	)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	if err := h.Shutdown(ctx); err != nil {
		b.Fatal(err)
	}
}
