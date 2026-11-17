// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect_test

import (
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestInList(t *testing.T) {
	t.Parallel()
	require.Equal(t, "DELETE FROM t WHERE k IN (?)", sqldialect.InList("DELETE FROM t WHERE k IN (%s)", 1))
	require.Equal(t, "DELETE FROM t WHERE k IN (?, ?, ?)", sqldialect.InList("DELETE FROM t WHERE k IN (%s)", 3))
}

func TestQueryKeys(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"k"}, Rows: [][]driver.Value{{[]byte("a")}, {[]byte("b")}}}
	})
	keys, err := sqldialect.QueryKeys(t.Context(), db, 2, "SELECT k FROM t LIMIT ?", 2)
	require.NoError(t, err)
	require.Equal(t, []any{[]byte("a"), []byte("b")}, keys)
	require.Equal(t, 3, cap(keys), "room for the keys and one more argument")
	require.Equal(t, []any{int64(2)}, fake.Calls()[0].Args)
	require.Equal(t, 1, fake.RowsClosed())
}

func TestQueryKeysError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: boom}
	})
	keys, err := sqldialect.QueryKeys(t.Context(), db, 1, "SELECT k FROM t")
	require.ErrorIs(t, err, boom)
	require.Nil(t, keys)
}
