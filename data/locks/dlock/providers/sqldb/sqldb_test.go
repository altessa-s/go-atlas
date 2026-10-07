// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var (
	errBoom         = errors.New("boom")
	numberedParamRE = regexp.MustCompile(`\$\d+`)
)

func TestNew_Validation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New(db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName("a.b.c"))
	require.ErrorIs(t, err, sqldb.ErrInvalidTableName)
	for _, opts := range [][]sqldb.Option{
		{sqldb.WithRenewRatio(1)},
		{sqldb.WithRenewRatio(1e-20)},
		{sqldb.WithTTL(500 * time.Microsecond)},
		{sqldb.WithTTL(2 * time.Millisecond), sqldb.WithRenewRatio(0.1)},
	} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, opts...)
		require.Error(t, err)
	}
}

func TestLock_KeyTooLong(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	l, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)
	_, err = l.Lock(t.Context(), strings.Repeat("ü", sqldb.MaxKeyLength+1))
	require.ErrorIs(t, err, sqldb.ErrValueTooLong)
	require.Empty(t, fake.Calls())
}

// TestPostgresStatements pins the acquisition as one statement on the
// database clock, with numbered placeholders.
func TestPostgresStatements(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"fencing"}} // held by someone else
	})
	l, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTTL(2*time.Second))
	require.NoError(t, err)

	_, err = l.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	q := calls[0].Query
	require.Contains(t, q, "ON CONFLICT (lock_key) DO UPDATE")
	require.Contains(t, q, "WHERE cur.expires_at <= CAST(EXTRACT(EPOCH FROM clock_timestamp())")
	require.Contains(t, q, "RETURNING fencing")
	require.NotContains(t, q, "?")
	require.Len(t, numberedParamRE.FindAllString(q, -1), 4)
	require.Equal(t, []any{"k", calls[0].Args[1], int64(2_000_000), int64(2_000_000)}, calls[0].Args)
}

// TestMySQLAcquire pins the MySQL acquisition: the row is created on first
// use, then taken over in one transaction that reads the new token only when
// the conditional UPDATE won.
func TestMySQLAcquire(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		affected int64
	}{
		{name: "won", affected: 1},
		{name: "held", affected: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
				switch {
				case strings.HasPrefix(query, "UPDATE"):
					return testhelpers.FakeSQLReply{Affected: tc.affected}
				case strings.HasPrefix(query, "SELECT fencing"):
					return testhelpers.FakeSQLReply{Columns: []string{"fencing"}, Rows: [][]driver.Value{{int64(7)}}}
				}
				return testhelpers.FakeSQLReply{Affected: 1}
			})
			l, err := sqldb.New(db, sqldb.DialectMySQL)
			require.NoError(t, err)
			t.Cleanup(func() { _ = l.Close(t.Context()) })

			lk, err := l.Lock(t.Context(), "k")
			calls := fake.Calls()
			require.Contains(t, calls[0].Query, "ON DUPLICATE KEY UPDATE lock_key = lock_key")
			require.False(t, calls[0].InTx)
			require.Equal(t, []any{[]byte("k")}, calls[0].Args)
			require.Contains(t, calls[1].Query, "UTC_TIMESTAMP(6)")
			require.True(t, calls[1].InTx)
			require.NotRegexp(t, numberedParamRE, calls[1].Query)
			if tc.affected == 0 {
				require.ErrorIs(t, err, errs.ErrLockNotHeld)
				require.Len(t, calls, 2)
				require.Zero(t, fake.Commits())
				return
			}
			require.NoError(t, err)
			require.Contains(t, calls[2].Query, "SELECT fencing")
			require.True(t, calls[2].InTx)
			require.Equal(t, 1, fake.Commits())
			require.NotNil(t, lk)
		})
	}
}

// A failed acquisition is reported and its possibly applied lease released by
// owner, without a fencing condition.
func TestLock_FailedAcquisitionReleasesByOwner(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(query, "INSERT") {
			return testhelpers.FakeSQLReply{Err: errBoom}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	l, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)

	_, err = l.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errBoom)
	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.Contains(t, calls[1].Query, "SET owner = ''")
	require.NotContains(t, calls[1].Query, "fencing")
	require.Equal(t, calls[0].Args[1], calls[1].Args[1], "released by the attempt's owner id")
}

func TestEnsureSchema(t *testing.T) {
	t.Parallel()
	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		l, err := sqldb.New(db, sqldb.DialectPostgres)
		require.NoError(t, err)
		require.NoError(t, l.EnsureSchema(t.Context()))
		calls := fake.Calls()
		require.Len(t, calls, 2)
		require.Contains(t, calls[0].Query, "pg_advisory_xact_lock")
		require.Contains(t, calls[1].Query, `CREATE TABLE IF NOT EXISTS "dlocks"`)
		require.Contains(t, calls[1].Query, `lock_key    VARCHAR(255) COLLATE "C" PRIMARY KEY`)
	})
	t.Run("mysql", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		l, err := sqldb.New(db, sqldb.DialectMySQL)
		require.NoError(t, err)
		require.NoError(t, l.EnsureSchema(t.Context()))
		q := fake.Calls()[0].Query
		require.Contains(t, q, "lock_key    VARBINARY(1020) NOT NULL PRIMARY KEY")
		require.NotRegexp(t, `\b(VARCHAR|TEXT)\b`, q)
	})
}

func TestProbe(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	l, err := sqldb.New(db, sqldb.DialectPostgres)
	require.NoError(t, err)
	require.NoError(t, l.Probe(t.Context()))
	require.NoError(t, l.Close(t.Context()))
	require.Error(t, l.Probe(t.Context()))
	_, err = l.Lock(t.Context(), "k")
	require.Error(t, err)
}
