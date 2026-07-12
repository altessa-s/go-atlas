// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"
)

func BenchmarkCloneRecord(b *testing.B) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "benchmark message", 0)
	r.AddAttrs(
		slog.String("key", "value"),
		slog.Int("count", 42),
	)

	b.ReportAllocs()
	for b.Loop() {
		_ = base.CloneRecord(r)
	}
}

func BenchmarkGroupPath(b *testing.B) {
	bs := base.NewBase(slog.NewTextHandler(io.Discard, nil))
	bs = bs.WithGroupBase("user")
	bs = bs.WithGroupBase("profile")

	b.ReportAllocs()
	for b.Loop() {
		_ = bs.GroupPath("name")
	}
}

func BenchmarkCloneGroups(b *testing.B) {
	groups := []string{"user", "profile", "settings"}

	b.ReportAllocs()
	for b.Loop() {
		_ = base.CloneGroups(groups)
	}
}
