// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// BenchmarkHistoryPaginated measures building a filtered page query — the
// translation and statement assembly — with the round trip cut off at the
// connection.
func BenchmarkHistoryPaginated(b *testing.B) {
	store, _ := mustNew(b)
	f := testhelpers.MustParseFilter(b, `success && startedAt >= 10 && error.startsWith("timeout")`)
	pg := scheduler.HistoryPagination{Pagination: scheduler.Pagination{AfterID: "h9", Limit: 100}, AfterStartedAt: 50}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = store.HistoryPaginated(ctx, "task", pg, f)
	}
}
