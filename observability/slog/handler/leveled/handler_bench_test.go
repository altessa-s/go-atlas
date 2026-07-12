// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leveled

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// benchRecord returns a representative record tagged with the given subsystem.
func benchRecord(subsystem string) slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	r.AddAttrs(
		slog.String(DefaultSubsystemKey, subsystem),
		slog.String("key", "value"),
	)
	return r
}

func BenchmarkHandle_Enabled(b *testing.B) {
	h := NewHandler(
		slog.NewTextHandler(io.Discard, nil),
		WithSubsystemLevels(map[string]slog.Level{"noisy": slog.LevelError}),
	)
	rec := benchRecord("app") // default level Info => forwarded
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandle_FilteredOut(b *testing.B) {
	h := NewHandler(
		slog.NewTextHandler(io.Discard, nil),
		WithSubsystemLevels(map[string]slog.Level{"noisy": slog.LevelError}),
	)
	rec := benchRecord("noisy") // Info < Error => dropped before the inner handler
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandle_CapturedSubsystem(b *testing.B) {
	h := NewHandler(slog.NewTextHandler(io.Discard, nil)).
		WithAttrs([]slog.Attr{slog.String(DefaultSubsystemKey, "app")})
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if err := h.Handle(ctx, rec); err != nil {
			b.Fatal(err)
		}
	}
}
