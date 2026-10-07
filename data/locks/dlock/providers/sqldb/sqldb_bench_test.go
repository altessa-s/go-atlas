// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// benchLocker answers every acquisition as held and every read with a lease,
// so the benchmarks measure statement building, binding and scanning.
func benchLocker(b *testing.B) *sqldb.Locker {
	b.Helper()
	db, fake := testhelpers.NewFakeSQL(b, func(query string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(query, "SELECT") {
			return testhelpers.FakeSQLReply{Columns: []string{"lock_key", "owner", "fencing", "acquired_at", "renewed_at", "ttl_us"},
				Rows: [][]driver.Value{{"k", "owner", int64(3), int64(1), int64(2), int64(10_000_000)}}}
		}
		return testhelpers.FakeSQLReply{Columns: []string{"fencing"}}
	})
	fake.DisableRecording()
	l, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(b, err)
	return l
}

func BenchmarkLockContended(b *testing.B) {
	l := benchLocker(b)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.Lock(ctx, "k"); err != errs.ErrLockNotHeld { //nolint:errorlint // the sentinel is returned unwrapped
			b.Fatal(err)
		}
	}
}

func BenchmarkGetLockInfo(b *testing.B) {
	l := benchLocker(b)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.GetLockInfo(ctx, "k"); err != nil {
			b.Fatal(err)
		}
	}
}
