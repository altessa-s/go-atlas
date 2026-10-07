// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// benchRow is a stored instance as the fake database returns it.
var benchRow = []driver.Value{"order-1", "place-order", "RUNNING", int64(2), "[0,3]", []byte(`{"n":1}`),
	`[{"name":"reserve","stage":0,"status":"COMPLETED","attempts":1}]`, base.UnixNano(), base.UnixNano(), int64(0),
	"owner", base.UnixNano(), int64(4), ""}

func benchStore(b *testing.B, d sqldb.Dialect) *sqldb.Store {
	b.Helper()
	db, fake := testhelpers.NewFakeSQL(b, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{benchRow}, Affected: 1}
	})
	fake.DisableRecording()
	store, err := sqldb.New(db, d)
	require.NoError(b, err)
	return store
}

func BenchmarkGet(b *testing.B) {
	store := benchStore(b, sqldb.DialectPostgres)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.Get(ctx, "order-1"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	for _, d := range dialects {
		b.Run(string(d), func(b *testing.B) {
			store := benchStore(b, d)
			ctx := b.Context()
			inst := fullInstance()
			b.ReportAllocs()
			for b.Loop() {
				if err := store.Update(ctx, inst); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFetchRecoverable(b *testing.B) {
	store := benchStore(b, sqldb.DialectPostgres)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := store.FetchRecoverable(ctx, base, 100); err != nil {
			b.Fatal(err)
		}
	}
}
