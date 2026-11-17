// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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
			s, fake := newStorage(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply {
				return testhelpers.FakeSQLReply{Err: tc.err}
			}, sqldb.WithTableName("app.events"))
			require.ErrorIs(t, s.EnsureSchema(t.Context()), tc.err)
			calls := fake.Calls()
			require.Len(t, calls, 1)
			require.True(t, strings.HasPrefix(calls[0].Query, "CREATE TABLE IF NOT EXISTS `app`.`events`"))
			require.Contains(t, calls[0].Query, "INDEX `events_trace_idx` (trace_id)")
		})
	}
}
