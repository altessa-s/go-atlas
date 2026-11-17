// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
)

// MySQL declares every index inline in the one CREATE TABLE statement.
func TestEnsureSchemaMySQLIndexesInline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "created"},
		{name: "DDL failure", err: errBoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
				return testhelpers.FakeSQLReply{Err: tc.err}
			})
			store, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTableName("app.instances"))
			require.NoError(t, err)
			require.ErrorIs(t, store.EnsureSchema(t.Context()), tc.err)
			calls := fake.Calls()
			require.Len(t, calls, 1)
			require.True(t, strings.HasPrefix(calls[0].Query, "CREATE TABLE IF NOT EXISTS `app`.`instances`"))
			require.Contains(t, calls[0].Query, "INDEX `instances_deadline_idx` (status, deadline)")
		})
	}
}
