// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
)

// noopHandler accepts every record and discards it, so enabled-path benchmarks
// measure attribute construction rather than handler formatting.
type noopHandler struct{}

func (noopHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (noopHandler) Handle(context.Context, slog.Record) error { return nil }
func (noopHandler) WithAttrs([]slog.Attr) slog.Handler        { return noopHandler{} }
func (noopHandler) WithGroup(string) slog.Handler             { return noopHandler{} }

func BenchmarkShouldIgnore(b *testing.B) {
	bs := NewWithFilter("bench", "middleware", "path",
		[]string{"/health"},
		[]*regexp.Regexp{regexp.MustCompile(`^/internal`)},
		nil,
	)

	b.Run("match", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			bs.ShouldIgnore("/health")
		}
	})
	b.Run("no_match", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			bs.ShouldIgnore("/api/v1/users")
		}
	})
}

func BenchmarkLogDebug(b *testing.B) {
	ctx := b.Context()
	attrs := []slog.Attr{slog.String("k", "v")}

	b.Run("disabled", func(b *testing.B) {
		logger := slog.New(levelHandler{captureHandler: &captureHandler{}, min: slog.LevelInfo})
		bs := New("bench", "middleware", "path", logger)
		b.ReportAllocs()
		for b.Loop() {
			bs.LogDebug(ctx, "msg", "/api", attrs...)
		}
	})
	b.Run("enabled", func(b *testing.B) {
		logger := slog.New(noopHandler{})
		bs := New("bench", "middleware", "path", logger)
		b.ReportAllocs()
		for b.Loop() {
			bs.LogDebug(ctx, "msg", "/api", attrs...)
		}
	})
}

func BenchmarkLogIgnored_Disabled(b *testing.B) {
	logger := slog.New(levelHandler{captureHandler: &captureHandler{}, min: slog.LevelInfo})
	bs := New("bench", "middleware", "path", logger)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		bs.LogIgnored(ctx, "/health")
	}
}
