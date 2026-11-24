// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type beforeSend struct{ error }

func (beforeSend) SafeToRetry() bool { return true }

type afterSend struct{ cause error }

func (e afterSend) Error() string   { return "response lost: " + e.cause.Error() }
func (e afterSend) Unwrap() error   { return e.cause }
func (afterSend) SafeToRetry() bool { return false }

func TestStatementRetryBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		attempts int
	}{
		{"explicitly unsafe", afterSend{driver.ErrBadConn}, 1},
		{"bad connection", driver.ErrBadConn, 2},
		{"joined safe failures", errors.Join(driver.ErrBadConn, beforeSend{errors.New("closed")}), 2},
		{"safe failure", fmt.Errorf("query: %w", beforeSend{errors.New("closed")}), 2},
		{"joined ambiguous outcome", fmt.Errorf("driver: %w", errors.Join(beforeSend{errors.New("cleanup write failed")}, errors.New("statement outcome unknown"))), 1},
		{"ambiguous failure", errors.New("connection lost while waiting for response"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: tc.err} })
			pool := sqldialect.Postgres.WrapPool(db)
			_, err := pool.ExecContext(t.Context(), "INSERT INTO events VALUES (1)")
			require.ErrorIs(t, err, tc.err)
			require.Len(t, fake.Calls(), tc.attempts)
			_, err = pool.QueryContext(t.Context(), "SELECT id FROM events")
			require.ErrorIs(t, err, tc.err)
			require.Len(t, fake.Calls(), 2*tc.attempts)
			var id int
			err = pool.QueryRowContext(t.Context(), "SELECT 1").Scan(&id)
			require.ErrorIs(t, err, tc.err)
			require.Len(t, fake.Calls(), 3*tc.attempts)
			if tc.attempts == 2 {
				require.Zero(t, db.Stats().OpenConnections)
			}
		})
	}
}

func TestCanceledQueryReleasesLease(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprintf("single=%t", single), func(t *testing.T) {
			db, _ := testhelpers.NewFakeSQL(t, nil)
			db.SetMaxOpenConns(1)
			pool := sqldialect.Postgres.WrapPool(db)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if single {
				row := pool.QueryRowContext(ctx, "SELECT 1")
				defer func() { require.ErrorIs(t, row.Scan(), context.Canceled) }()
			} else {
				rows, err := pool.QueryContext(ctx, "SELECT 1")
				require.NoError(t, err)
				defer func() { require.NoError(t, rows.Close()) }()
			}
			cancel()
			require.Eventually(t, func() bool { return db.Stats().InUse == 0 }, time.Second, time.Millisecond)
		})
	}
}

func TestCancellationStopsPreSendRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		cancel()
		return testhelpers.FakeSQLReply{Err: driver.ErrBadConn}
	})
	_, err := sqldialect.Postgres.WrapPool(db).ExecContext(ctx, "INSERT INTO events VALUES (1)")
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, fake.Calls(), 1)
	require.Zero(t, db.Stats().OpenConnections)
}

func TestDriverRetryBudget(t *testing.T) {
	ctx := sqldialect.StatementContext(t.Context())
	failure := beforeSend{errors.New("connection closed before write")}
	require.ErrorIs(t, sqldialect.DriverError(ctx, failure), driver.ErrBadConn)
	final := sqldialect.DriverError(ctx, driver.ErrBadConn)
	require.NotErrorIs(t, final, driver.ErrBadConn)
	require.True(t, sqldialect.SafeToRetry(final))
	require.ErrorIs(t, sqldialect.DriverError(ctx, failure), failure)
	independent := sqldialect.StatementContext(ctx)
	require.ErrorIs(t, sqldialect.DriverError(independent, failure), driver.ErrBadConn)
	canceled, cancel := context.WithCancel(independent)
	cancel()
	require.ErrorIs(t, sqldialect.DriverError(canceled, failure), failure)
	ambiguous := errors.New("waiting for result")
	require.ErrorIs(t, sqldialect.DriverError(ctx, ambiguous), ambiguous)
}

func TestSharedStatementContextAllowsOneRetry(t *testing.T) {
	ctx := sqldialect.StatementContext(t.Context())
	var retries atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		wg.Go(func() {
			<-start
			if errors.Is(sqldialect.DriverError(ctx, driver.ErrBadConn), driver.ErrBadConn) {
				retries.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, retries.Load())
}

func TestAmbiguousJoinedFailureDoesNotReplayWrite(t *testing.T) {
	failure := fmt.Errorf("write: %w", errors.Join(driver.ErrBadConn, errors.New("response lost")))
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: failure}
	})
	_, err := sqldialect.Postgres.WrapPool(db).ExecContext(t.Context(), "INSERT INTO events VALUES (1)")
	require.ErrorIs(t, err, failure)
	require.Len(t, fake.Calls(), 1)
}

func TestStandaloneBadConnections(t *testing.T) {
	attempts := 0
	db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		attempts++
		if attempts <= 2 {
			return testhelpers.FakeSQLReply{Err: driver.ErrBadConn}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	pool := sqldialect.MySQL.WrapPool(db)
	_, err := pool.ExecContext(t.Context(), "INSERT INTO events VALUES (1)")
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestExhaustedRowsReleaseConnection(t *testing.T) {
	db, _ := testhelpers.NewFakeSQL(t, nil)
	rows, err := sqldialect.Postgres.WrapPool(db).QueryContext(t.Context(), "SELECT 1")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.Zero(t, db.Stats().InUse)
}
