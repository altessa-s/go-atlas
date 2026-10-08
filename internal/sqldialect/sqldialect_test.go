// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

func TestTable(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "t; DROP TABLE x", "a.b.c", "1tasks", `"quoted"`, "tab le", ".t", "s.", "a..b", "s.1t", "t\n", "t-x", "ü",
		strings.Repeat("t", sqldialect.MaxIdentLen+1), "s." + strings.Repeat("t", sqldialect.MaxIdentLen+1)} {
		_, err := sqldialect.Postgres.Table(name)
		require.ErrorIs(t, err, sqldialect.ErrInvalidTableName, name)
	}
	for _, tc := range []struct {
		style      sqldialect.Style
		name, want string
	}{
		{sqldialect.Postgres, "events", `"events"`},
		{sqldialect.Postgres, "app.events", `"app"."events"`},
		{sqldialect.MySQL, "app.events", "`app`.`events`"},
		{sqldialect.MySQL, "_T1", "`_T1`"},
		{sqldialect.Postgres, "a1_.B_2", `"a1_"."B_2"`},
	} {
		got, err := tc.style.Table(tc.name)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got)
	}
	long := strings.Repeat("t", sqldialect.MaxIdentLen)
	_, err := sqldialect.MySQL.Table(long + "." + long)
	require.NoError(t, err, "the limit applies per part and is inclusive")
}

func TestIndexName(t *testing.T) {
	t.Parallel()
	require.Equal(t, `"tasks_due_idx"`, sqldialect.Postgres.IndexName("tasks", "due"))
	require.Equal(t, "`tasks_due_idx`", sqldialect.MySQL.IndexName("tasks", "due"))

	long := strings.Repeat("t", 62)
	a, b := sqldialect.Postgres.IndexName(long+"a", "due"), sqldialect.Postgres.IndexName(long+"b", "due")
	require.NotEqual(t, a, b, "truncated names must stay distinct")
	for _, n := range []string{a, b} {
		unquoted := strings.Trim(n, `"`)
		require.LessOrEqual(t, len(unquoted), sqldialect.MaxIdentLen)
		require.True(t, strings.HasPrefix(unquoted, "tttt"), "a readable prefix is kept")
		require.True(t, strings.HasSuffix(unquoted, "_due_idx"))
	}
	require.Equal(t, a, sqldialect.Postgres.IndexName(long+"a", "due"), "the name is deterministic")
}

func TestBind(t *testing.T) {
	t.Parallel()
	q := "UPDATE t SET a = ? WHERE id = ? AND b = ?"
	require.Equal(t, "UPDATE t SET a = $1 WHERE id = $2 AND b = $3", sqldialect.Postgres.Bind(q, 1))
	require.Equal(t, "UPDATE t SET a = $9 WHERE id = $10 AND b = $11", sqldialect.Postgres.Bind(q, 9))
	require.Equal(t, q, sqldialect.MySQL.Bind(q, 5))
	require.Equal(t, "SELECT 1", sqldialect.Postgres.Bind("SELECT 1", 1))
}

func TestQualifiedParts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, table, schema string }{
		{"events", "events", ""},
		{"app.events", "events", "app"},
	} {
		require.Equal(t, tc.table, sqldialect.Unqualified(tc.name))
		require.Equal(t, tc.schema, sqldialect.Qualifier(tc.name))
	}
}
