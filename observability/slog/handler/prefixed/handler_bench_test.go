// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prefixed_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"
)

// benchRecord returns a representative record. When withPrefix is true the
// record carries an attribute matching the configured prefix key.
func benchRecord(withPrefix bool) slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	if withPrefix {
		r.AddAttrs(slog.String("subsystem", "api"))
	}
	r.AddAttrs(
		slog.String("key", "value"),
		slog.Int("count", 42),
	)
	return r
}

func BenchmarkHandle_WithPrefixAttr(b *testing.B) {
	h := prefixed.NewHandler(
		slog.NewTextHandler(io.Discard, nil),
		prefixed.WithPrefix("subsystem"),
	)
	rec := benchRecord(true)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandle_NoPrefix(b *testing.B) {
	h := prefixed.NewHandler(
		slog.NewTextHandler(io.Discard, nil),
		prefixed.WithPrefix("subsystem"),
	)
	rec := benchRecord(false)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}
