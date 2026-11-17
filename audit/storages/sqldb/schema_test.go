// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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
			s, fake := newStorage(t, sqldb.DialectMySQL, func(q string, _ []any) testhelpers.FakeSQLReply {
				switch {
				case strings.HasPrefix(q, "SELECT"):
					return testhelpers.FakeSQLReply{Columns: []string{"index_name"}, Rows: [][]driver.Value{
						{"PRIMARY"}, {"events_ts_idx"}, {"events_actor_idx"}, {"events_resource_idx"}, {"events_request_idx"},
					}}
				case strings.HasPrefix(q, "ALTER"):
					return testhelpers.FakeSQLReply{Err: tc.err}
				default:
					return testhelpers.FakeSQLReply{}
				}
			}, sqldb.WithTableName("app.events"))
			require.ErrorIs(t, s.EnsureSchema(t.Context()), tc.err)
			calls := fake.Calls()
			require.Len(t, calls, 3)
			require.Equal(t, []any{"app", "events"}, calls[1].Args)
			require.Equal(t, "ALTER TABLE `app`.`events` ADD INDEX `events_trace_idx` (trace_id)", calls[2].Query)
		})
	}
}
