// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagait_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
)

// TestEnsureSchemaRestoresMySQLIndexes simulates an older table without the
// deadline index, then checks both recovery indexes after startup.
func TestEnsureSchemaRestoresMySQLIndexes(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		if target.dialect != sqldb.DialectMySQL {
			continue
		}
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.open(t)
			store, err := sqldb.New(db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(t.Context()))
			index := sqldialect.MySQL.IndexName(table, "deadline")
			_, err = db.ExecContext(t.Context(), "ALTER TABLE "+sqldialect.MySQL.Ident(table)+" DROP INDEX "+index)
			require.NoError(t, err)
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(),
				"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
				table, strings.Trim(index, "`")).Scan(&count))
			require.Zero(t, count)

			var schema string
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&schema))
			store, err = sqldb.New(db, target.dialect, sqldb.WithTableName(schema+"."+table))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(t.Context()))
			require.NoError(t, store.EnsureSchema(t.Context()), "reconciliation is idempotent")
			for suffix, columns := range map[string]string{"lease": "status, lease_until", "deadline": "status, deadline"} {
				var got string
				require.NoError(t, db.QueryRowContext(t.Context(),
					"SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ') FROM information_schema.statistics"+
						" WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
					table, strings.Trim(sqldialect.MySQL.IndexName(table, suffix), "`")).Scan(&got))
				require.Equal(t, columns, got, "index %s", suffix)
			}
		})
	}
}
