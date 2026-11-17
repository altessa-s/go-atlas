// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestAddMissingMySQLIndexes(t *testing.T) {
	t.Parallel()
	for _, schema := range []string{"", "app"} {
		t.Run("schema="+schema, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(q, "SELECT") {
					return testhelpers.FakeSQLReply{Columns: []string{"index_name"}, Rows: [][]driver.Value{
						{"PRIMARY"}, {"events_ts_idx"}, {"events_actor_idx"},
					}}
				}
				return testhelpers.FakeSQLReply{}
			})
			// DDL must release catalog rows first, even when the pool has one connection.
			db.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			indexes := []sqldialect.MySQLIndex{
				{Name: sqldialect.MySQL.IndexName("events", "ts"), Columns: "ts_ms, id"},
				{Name: "events_actor_idx", Columns: "actor_id, ts_ms, id"},
				{Name: sqldialect.MySQL.IndexName("events", "trace"), Columns: "trace_id"},
				{Name: "events_request_idx", Columns: "request_id"},
			}
			require.NoError(t, sqldialect.AddMissingMySQLIndexes(ctx, db, schema, "events", indexes))
			calls := fake.Calls()
			require.Len(t, calls, 3, "only the two missing indexes are added")
			require.Equal(t, "SELECT DISTINCT index_name FROM information_schema.statistics"+
				" WHERE table_schema = COALESCE(?, DATABASE()) AND table_name = ?", calls[0].Query)
			var qualifier any
			table := "`events`"
			if schema != "" {
				qualifier = schema
				table = "`app`." + table
			}
			require.Equal(t, []any{qualifier, "events"}, calls[0].Args)
			require.Equal(t, "ALTER TABLE "+table+" ADD INDEX `events_trace_idx` (trace_id)", calls[1].Query)
			require.Equal(t, "ALTER TABLE "+table+" ADD INDEX `events_request_idx` (request_id)", calls[2].Query)
			require.Equal(t, 1, fake.RowsClosed())
			for _, call := range calls {
				require.False(t, call.InTx)
			}
		})
	}
}

func TestAddMissingMySQLIndexesAlreadyPresent(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"index_name"}, Rows: [][]driver.Value{{"events_ts_idx"}}}
	})
	require.NoError(t, sqldialect.AddMissingMySQLIndexes(t.Context(), db, "", "events", []sqldialect.MySQLIndex{
		{Name: sqldialect.MySQL.IndexName("events", "ts"), Columns: "ts_ms, id"},
	}))
	require.Len(t, fake.Calls(), 1, "an up-to-date table needs no DDL")
}

func TestAddMissingMySQLIndexesErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	other := &mysqlError{Number: 1062}
	for _, tc := range []struct {
		name     string
		queryErr error
		alterErr error
		wantErr  error
		calls    int
	}{
		{name: "catalog failure", queryErr: boom, wantErr: boom, calls: 1},
		{name: "DDL failure", alterErr: boom, wantErr: boom, calls: 2},
		{name: "other server error", alterErr: other, wantErr: other, calls: 2},
		{name: "concurrent creation", alterErr: &mysqlError{Number: 1061}, calls: 3},
		{name: "wrapped concurrent creation", alterErr: fmt.Errorf("add: %w", &mysqlError{Number: 1061}), calls: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(q, "SELECT") {
					return testhelpers.FakeSQLReply{Err: tc.queryErr}
				}
				return testhelpers.FakeSQLReply{Err: tc.alterErr}
			})
			err := sqldialect.AddMissingMySQLIndexes(t.Context(), db, "", "events", []sqldialect.MySQLIndex{
				{Name: "first_idx", Columns: "id"}, {Name: "second_idx", Columns: "ts_ms"},
			})
			require.ErrorIs(t, err, tc.wantErr)
			require.Len(t, fake.Calls(), tc.calls)
		})
	}
}

func TestAddMissingMySQLIndexesScanError(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"index_name"}, Rows: [][]driver.Value{{nil}}}
	})
	require.Error(t, sqldialect.AddMissingMySQLIndexes(t.Context(), db, "", "events", []sqldialect.MySQLIndex{
		{Name: "events_idx", Columns: "id"},
	}))
	require.Len(t, fake.Calls(), 1, "a catalog scan error prevents DDL")
	require.Equal(t, 1, fake.RowsClosed())
}

func TestAddMissingMySQLIndexesInvalidTable(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	require.ErrorIs(t, sqldialect.AddMissingMySQLIndexes(t.Context(), db, "", "bad table", nil), sqldialect.ErrInvalidTableName)
	require.Empty(t, fake.Calls())
}

func TestExecPostgresDDL(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	stmts := []string{"CREATE TABLE IF NOT EXISTS a ()", "CREATE INDEX IF NOT EXISTS i ON a ()"}
	require.NoError(t, sqldialect.ExecPostgresDDL(t.Context(), db, []string{"b", "a", "b"}, stmts))

	calls := fake.Calls()
	require.Len(t, calls, 4, "one lock per distinct name, then the statements")
	for _, c := range calls {
		require.True(t, c.InTx, c.Query)
	}
	require.Equal(t, "SELECT pg_advisory_xact_lock($1)", calls[0].Query)
	require.Equal(t, "SELECT pg_advisory_xact_lock($1)", calls[1].Query)
	first, second := calls[0].Args[0].(int64), calls[1].Args[0].(int64)
	require.Less(t, first, second, "locks are taken in key order")
	require.Equal(t, stmts, []string{calls[2].Query, calls[3].Query})
	require.Equal(t, 1, fake.Commits())

	// The same names map to the same keys in every process.
	db2, fake2 := testhelpers.NewFakeSQL(t, nil)
	require.NoError(t, sqldialect.ExecPostgresDDL(t.Context(), db2, []string{"a", "b"}, nil))
	require.Equal(t, []any{first}, fake2.Calls()[0].Args)
	require.Equal(t, []any{second}, fake2.Calls()[1].Args)
}

func TestExecPostgresDDLRollsBackOnFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if q == "BAD" {
			return testhelpers.FakeSQLReply{Err: boom}
		}
		return testhelpers.FakeSQLReply{}
	})
	err := sqldialect.ExecPostgresDDL(t.Context(), db, []string{"t"}, []string{"OK", "BAD", "NEVER"})
	require.ErrorIs(t, err, boom)
	require.Equal(t, 0, fake.Commits())
	require.Equal(t, 1, fake.Rollbacks())
	require.Len(t, fake.Calls(), 3, "statements after the failure do not run")
}
