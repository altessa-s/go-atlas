// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
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
	require.ErrorIs(t, err, providers.ErrLockNotHeld)
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
				require.ErrorIs(t, err, providers.ErrLockNotHeld)
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

// TestMySQLRenewLocksRowFirst pins that a MySQL renewal locks the row and
// only then compares the clock, in one transaction: UTC_TIMESTAMP is fixed
// when a statement starts, so a renewal that waited for the row lock must not
// read the clock in the statement that waited.
func TestMySQLRenewLocksRowFirst(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "SELECT fencing"):
			return testhelpers.FakeSQLReply{Columns: []string{"fencing"}, Rows: [][]driver.Value{{int64(1)}}}
		case strings.HasPrefix(query, "SELECT 1"):
			return testhelpers.FakeSQLReply{Columns: []string{"1"}, Rows: [][]driver.Value{{int64(1)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	l, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTTL(60*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	_, err = l.Lock(t.Context(), "k")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		calls := fake.Calls()
		for i, c := range calls {
			if strings.Contains(c.Query, "SET renewed_at") {
				prev := calls[i-1]
				return c.InTx && prev.InTx && strings.HasSuffix(prev.Query, "FOR UPDATE")
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond, "the renewal must follow a row lock in its transaction")
}

func TestNew_HardLeaseAndCleanupValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	_, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithExpiredCleanup(-time.Second))
	require.Error(t, err, "a negative cleanup grace is refused")
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithExpiredCleanup(0))
	require.NoError(t, err, "a zero grace deletes rows as soon as their lease ended")
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithoutRenewal(), sqldb.WithRenewRatio(1))
	require.NoError(t, err, "the renew ratio plays no part in a hard lease")
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithoutRenewal(), sqldb.WithTTL(500*time.Microsecond))
	require.Error(t, err, "a hard lease still needs a TTL of at least 1ms")
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithoutRenewal(), sqldb.WithTTL(time.Millisecond))
	require.NoError(t, err, "a hard lease needs no renewal interval")
}

// TestPurgeExpired pins the cleanup sweep: nothing without
// WithExpiredCleanup; on PostgreSQL one DELETE per batch on the database
// clock; on MySQL an unlocked read of the batch's keys, then a DELETE through
// them that repeats the condition. A full batch makes the sweep continue.
func TestPurgeExpired(t *testing.T) {
	t.Parallel()
	const grace = 1500 * time.Millisecond

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, nil)
		l, err := sqldb.New(db, sqldb.DialectPostgres)
		require.NoError(t, err)
		n, err := l.PurgeExpired(t.Context())
		require.NoError(t, err)
		require.Zero(t, n)
		require.Empty(t, fake.Calls())
	})
	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		affected := []int64{4, 3}
		db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
			n := affected[0]
			affected = affected[1:]
			return testhelpers.FakeSQLReply{Affected: n}
		})
		l, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithExpiredCleanup(grace), sqldb.WithPurgeBatchSize(4))
		require.NoError(t, err)
		n, err := l.PurgeExpired(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, 7, n)
		calls := fake.Calls()
		require.Len(t, calls, 2, "a full batch is followed by another")
		q := calls[0].Query
		require.Contains(t, q, `DELETE FROM "dlocks" WHERE lock_key IN (SELECT lock_key FROM "dlocks" WHERE expires_at <= CAST(`)
		require.Contains(t, q, "clock_timestamp()")
		require.Contains(t, q, "LIMIT 4 FOR UPDATE SKIP LOCKED")
		require.Equal(t, []any{int64(1_500_000)}, calls[0].Args)
	})
	t.Run("mysql", func(t *testing.T) {
		t.Parallel()
		db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
			if strings.HasPrefix(query, "SELECT") {
				return testhelpers.FakeSQLReply{Columns: []string{"lock_key"}, Rows: [][]driver.Value{{[]byte("a")}, {[]byte("b")}}}
			}
			return testhelpers.FakeSQLReply{Affected: 1}
		})
		l, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithExpiredCleanup(grace), sqldb.WithPurgeBatchSize(4))
		require.NoError(t, err)
		n, err := l.PurgeExpired(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, 1, n, "a row taken over meanwhile fails the repeated condition")
		calls := fake.Calls()
		require.Len(t, calls, 2, "a short batch ends the sweep")
		require.Contains(t, calls[0].Query, "UTC_TIMESTAMP(6)")
		require.Contains(t, calls[0].Query, "LIMIT 4")
		require.NotContains(t, calls[0].Query, "FOR UPDATE")
		require.Equal(t, []any{int64(1_500_000)}, calls[0].Args)
		require.Contains(t, calls[1].Query, "DELETE FROM `dlocks` WHERE lock_key IN (?, ?) AND expires_at <= ")
		require.Equal(t, []any{[]byte("a"), []byte("b"), int64(1_500_000)}, calls[1].Args)
	})
	t.Run("error", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
			return testhelpers.FakeSQLReply{Err: errBoom}
		})
		l, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithExpiredCleanup(0))
		require.NoError(t, err)
		_, err = l.PurgeExpired(t.Context())
		require.ErrorIs(t, err, errBoom)
	})
}

// TestMySQLAcquireRestartsAfterCleanup pins that a MySQL acquisition whose
// take finds the key's row deleted by the cleanup, between creating and taking
// it, starts over instead of reporting the key held.
func TestMySQLAcquireRestartsAfterCleanup(t *testing.T) {
	t.Parallel()
	var takes int
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "UPDATE"):
			takes++
			if takes == 1 {
				return testhelpers.FakeSQLReply{}
			}
			return testhelpers.FakeSQLReply{Affected: 1}
		case strings.HasPrefix(query, "SELECT 1"):
			return testhelpers.FakeSQLReply{Columns: []string{"1"}} // deleted
		case strings.HasPrefix(query, "SELECT fencing"):
			return testhelpers.FakeSQLReply{Columns: []string{"fencing"}, Rows: [][]driver.Value{{int64(1)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	l, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithExpiredCleanup(time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.WithoutCancel(t.Context())) })

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.NotNil(t, lk)
	var ensures int
	for _, c := range fake.Calls() {
		if strings.Contains(c.Query, "ON DUPLICATE KEY") {
			ensures++
		}
	}
	require.Equal(t, 2, ensures, "the row is created again")
}

// TestMySQLAcquireHeldWithCleanup pins that a take finding the row present
// reports the key held without starting over.
func TestMySQLAcquireHeldWithCleanup(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "UPDATE"):
			return testhelpers.FakeSQLReply{}
		case strings.HasPrefix(query, "SELECT 1"):
			return testhelpers.FakeSQLReply{Columns: []string{"1"}, Rows: [][]driver.Value{{int64(1)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	l, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithExpiredCleanup(time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.WithoutCancel(t.Context())) })
	_, err = l.Lock(t.Context(), "k")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)
	require.Len(t, fake.Calls(), 3)
}
