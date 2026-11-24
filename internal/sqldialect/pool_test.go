// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type beforeSend struct{ error }

func (beforeSend) SafeToRetry() bool { return true }

func TestStatementRetryBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		attempts int
	}{
		{"bad connection", driver.ErrBadConn, 2},
		{"safe failure", fmt.Errorf("query: %w", beforeSend{errors.New("closed")}), 2},
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

func TestDriverRetryBudget(t *testing.T) {
	ctx := sqldialect.StatementContext(t.Context())
	failure := beforeSend{errors.New("connection closed before write")}
	require.ErrorIs(t, sqldialect.DriverError(ctx, failure), driver.ErrBadConn)
	final := sqldialect.DriverError(ctx, driver.ErrBadConn)
	require.NotErrorIs(t, final, driver.ErrBadConn)
	require.True(t, sqldialect.SafeToRetry(final))
	independent := sqldialect.StatementContext(ctx)
	require.ErrorIs(t, sqldialect.DriverError(independent, failure), driver.ErrBadConn)
	canceled, cancel := context.WithCancel(independent)
	cancel()
	require.ErrorIs(t, sqldialect.DriverError(canceled, failure), failure)
	ambiguous := errors.New("waiting for result")
	require.ErrorIs(t, sqldialect.DriverError(ctx, ambiguous), ambiguous)
}
