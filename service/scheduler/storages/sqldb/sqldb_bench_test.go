// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
)

// BenchmarkTasksPaginatedFiltered measures the per-call work the storage adds
// around a filtered page query: translating the filter and assembling the
// statement (the fake driver makes the round trip itself negligible).
func BenchmarkTasksPaginatedFiltered(b *testing.B) {
	db, fake := testhelpers.NewFakeSQL(b, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Columns: taskCols()} })
	fake.DisableRecording()
	store, err := sqldb.New(db, sqldb.DialectPostgres)
	if err != nil {
		b.Fatal(err)
	}
	parser, err := filter.NewParser()
	if err != nil {
		b.Fatal(err)
	}
	node, err := parser.Parse(b.Context(), `status == 1 && priority in [1, 2] && description.contains("x")`)
	if err != nil {
		b.Fatal(err)
	}
	pg := scheduler.Pagination{AfterID: "k", Limit: 50}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.TasksPaginated(b.Context(), pg, node); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkClaimRun measures the hot dispatch path's statement and argument
// assembly.
func BenchmarkClaimRun(b *testing.B) {
	db, fake := testhelpers.NewFakeSQL(b, nil)
	fake.DisableRecording()
	store, err := sqldb.New(db, sqldb.DialectMySQL)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.ClaimRun(b.Context(), "task", scheduler.RunClaim{NextRunAt: 100, StartedAt: 100, RunID: "run"}); err != nil {
			b.Fatal(err)
		}
	}
}
