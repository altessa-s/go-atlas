// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
)

func benchStore(b *testing.B, d sqldb.Dialect, rows int) *sqldb.Store[payload] {
	b.Helper()
	result := make([][]driver.Value, rows)
	for i := range result {
		result[i] = row("k", retryqueue.StatePending)
	}
	db, fake := testhelpers.NewFakeSQL(b, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: columns, Rows: result, Affected: 1}
	})
	fake.DisableRecording()
	store, err := sqldb.New[payload](db, d)
	require.NoError(b, err)
	return store
}

func BenchmarkMark(b *testing.B) {
	for _, d := range dialects {
		b.Run(string(d), func(b *testing.B) {
			store := benchStore(b, d, 1)
			ctx := b.Context()
			item := retryqueue.Item[payload]{Key: "k", Generation: 1, Since: base, NextAttemptAt: base, Payload: payload{Note: "n"}}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := store.Mark(ctx, item); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkListDue(b *testing.B) {
	store := benchStore(b, sqldb.DialectPostgres, 100)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.ListDue(ctx, base, 100); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReschedule(b *testing.B) {
	store := benchStore(b, sqldb.DialectPostgres, 0)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if err := store.Reschedule(ctx, "k", 1, base); err != nil {
			b.Fatal(err)
		}
	}
}
