// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// BenchmarkBind measures placeholder renumbering, which the scheduler storage
// runs on every filtered page query.
func BenchmarkBind(b *testing.B) {
	q := " AND task_id = ? AND (started_at < ? OR (started_at = ? AND id < ?)) ORDER BY started_at DESC, id DESC LIMIT ?"
	b.ReportAllocs()
	for b.Loop() {
		_ = sqldialect.Postgres.Bind(q, 7)
	}
}

// BenchmarkIndexName measures index naming on the hashed path.
func BenchmarkIndexName(b *testing.B) {
	table := strings.Repeat("t", 70)
	b.ReportAllocs()
	for b.Loop() {
		_ = sqldialect.MySQL.IndexName(table, "task")
	}
}
