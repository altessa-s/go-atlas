// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

// benchDiscardHandler is a no-op inner handler so benchmarks measure the
// masking layer alone, not downstream encoding.
type benchDiscardHandler struct{}

func (benchDiscardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (benchDiscardHandler) Handle(context.Context, slog.Record) error { return nil }
func (benchDiscardHandler) WithAttrs([]slog.Attr) slog.Handler        { return benchDiscardHandler{} }
func (benchDiscardHandler) WithGroup(string) slog.Handler             { return benchDiscardHandler{} }

type benchInner struct {
	Host string
	Port int
}

// benchCleanRequest has no field whose name can match the configured masks.
type benchCleanRequest struct {
	RequestID string
	Method    string
	Path      string
	Attempts  int
	Inner     benchInner
}

// benchSensitiveRequest carries a field that must be masked.
type benchSensitiveRequest struct {
	User     string
	Password string
	Inner    benchInner
}

type benchItem struct {
	Name  string
	Value int
}

// benchSensitiveWithSlice forces the walk (Token matches) and exercises
// per-element slice handling.
type benchSensitiveWithSlice struct {
	Token string
	Items []benchItem
}

func benchRecord(attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "benchmark message", 0)
	r.AddAttrs(attrs...)
	return r
}

// BenchmarkHandlerHandle measures Handle end-to-end for the payload shapes
// the reflection walker deals with. The fields-only configuration (no
// patterns) is the common production shape for explicit mask lists.
func BenchmarkHandlerHandle(b *testing.B) {
	ctx := b.Context()

	fieldsOnly := masking.NewHandler(benchDiscardHandler{},
		masking.WithField("password", masking.FullMask()),
		masking.WithField("token", masking.FullMask()),
	)

	b.Run("clean_struct", func(b *testing.B) {
		rec := benchRecord(slog.Any("payload", benchCleanRequest{
			RequestID: "req-123",
			Method:    "GET",
			Path:      "/v1/items",
			Attempts:  2,
			Inner:     benchInner{Host: "localhost", Port: 8080},
		}))
		b.ReportAllocs()
		for b.Loop() {
			_ = fieldsOnly.Handle(ctx, rec)
		}
	})

	b.Run("sensitive_struct", func(b *testing.B) {
		rec := benchRecord(slog.Any("payload", benchSensitiveRequest{
			User:     "alice",
			Password: "hunter2",
			Inner:    benchInner{Host: "localhost", Port: 8080},
		}))
		b.ReportAllocs()
		for b.Loop() {
			_ = fieldsOnly.Handle(ctx, rec)
		}
	})

	b.Run("sensitive_with_slice", func(b *testing.B) {
		items := make([]benchItem, 8)
		for i := range items {
			items[i] = benchItem{Name: "item", Value: i}
		}
		rec := benchRecord(slog.Any("payload", benchSensitiveWithSlice{
			Token: "tok-abc",
			Items: items,
		}))
		b.ReportAllocs()
		for b.Loop() {
			_ = fieldsOnly.Handle(ctx, rec)
		}
	})

	b.Run("group_attrs", func(b *testing.B) {
		rec := benchRecord(slog.Group("req",
			slog.String("method", "GET"),
			slog.String("path", "/v1/items"),
			slog.Group("peer",
				slog.String("host", "localhost"),
				slog.Int("port", 8080),
			),
		))
		b.ReportAllocs()
		for b.Loop() {
			_ = fieldsOnly.Handle(ctx, rec)
		}
	})

	// WithDefaults registers regex patterns, which can match arbitrary
	// paths — the walker must stay active for every type there.
	b.Run("defaults_clean_struct", func(b *testing.B) {
		defaults := masking.NewHandler(benchDiscardHandler{}, masking.WithDefaults())
		rec := benchRecord(slog.Any("payload", benchCleanRequest{
			RequestID: "req-123",
			Method:    "GET",
			Path:      "/v1/items",
			Attempts:  2,
			Inner:     benchInner{Host: "localhost", Port: 8080},
		}))
		b.ReportAllocs()
		for b.Loop() {
			_ = defaults.Handle(ctx, rec)
		}
	})
}
