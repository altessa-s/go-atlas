// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func benchStorage(b *testing.B, rows int) *sqldb.Storage {
	b.Helper()
	payload, err := json.Marshal(event("e1"))
	require.NoError(b, err)
	page := make([][]driver.Value, rows)
	for i := range page {
		page[i] = []driver.Value{payload}
	}
	db, fake := testhelpers.NewFakeSQL(b, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"payload"}, Rows: page, Affected: 1}
	})
	fake.DisableRecording()
	s, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(b, err)
	return s
}

func BenchmarkStoreBatch(b *testing.B) {
	s := benchStorage(b, 0)
	batch := make([]*audit.Event, 100)
	for i := range batch {
		batch[i] = event(fmt.Sprintf("e%d", i))
	}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if err := s.StoreBatch(ctx, batch); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQueryPage(b *testing.B) {
	s := benchStorage(b, 50)
	ctx := b.Context()
	q := &audit.Query{ActorID: "u1", Limit: 50}
	b.ReportAllocs()
	for b.Loop() {
		for _, err := range s.Query(ctx, q) {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
