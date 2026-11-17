// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
)

func TestEnsureSchemaAddsMissingMySQLIndexes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "missing index"},
		{name: "DDL failure", err: errBoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
				switch {
				case strings.HasPrefix(q, "SELECT"):
					return testhelpers.FakeSQLReply{Columns: []string{"index_name"}, Rows: [][]driver.Value{
						{"PRIMARY"}, {"instances_lease_idx"},
					}}
				case strings.HasPrefix(q, "ALTER"):
					return testhelpers.FakeSQLReply{Err: tc.err}
				default:
					return testhelpers.FakeSQLReply{}
				}
			})
			store, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTableName("app.instances"))
			require.NoError(t, err)
			require.ErrorIs(t, store.EnsureSchema(t.Context()), tc.err)
			calls := fake.Calls()
			require.Len(t, calls, 3)
			require.Equal(t, []any{"app", "instances"}, calls[1].Args)
			require.Equal(t, "ALTER TABLE `app`.`instances` ADD INDEX `instances_deadline_idx` (status, deadline)", calls[2].Query)
		})
	}
}
