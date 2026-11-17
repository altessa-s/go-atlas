// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

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
