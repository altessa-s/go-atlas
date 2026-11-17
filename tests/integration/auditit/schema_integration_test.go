// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// TestEnsureSchemaCreatesMySQLIndexes checks the inline index definitions of
// the MySQL table, created through a schema-qualified name, and that a second
// EnsureSchema is a no-op.
func TestEnsureSchemaCreatesMySQLIndexes(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		if target.dialect != sqldb.DialectMySQL {
			continue
		}
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			_, db, table := target.open(t)
			var schema string
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&schema))
			st, err := sqldb.New(db, target.dialect, sqldb.WithTableName(schema+"."+table))
			require.NoError(t, err)
			require.NoError(t, st.EnsureSchema(t.Context()))
			require.NoError(t, st.EnsureSchema(t.Context()), "EnsureSchema is idempotent")
			for suffix, columns := range map[string]string{
				"ts": "ts_ms, id", "actor": "actor_id, ts_ms, id", "resource": "resource_type, resource_id, ts_ms, id",
				"request": "request_id", "trace": "trace_id",
			} {
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
