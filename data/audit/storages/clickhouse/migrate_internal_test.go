// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"context"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"
)

// migrateConn answers the schema queries and records every statement the
// migration executes.
func migrateConn(engine string, columns, indexes []string) *mockConn {
	conn := &mockConn{}
	conn.queryFn = func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "system.tables"):
			if engine == "" {
				return &mockNameRows{}, nil
			}

			return &mockNameRows{rows: [][]any{tableRow(engine, 1)}}, nil
		case strings.Contains(query, "data_skipping_indices"):
			return indexRows(indexes...), nil
		default:
			return nameRows(columns...), nil
		}
	}

	return conn
}

func newMigratingStorage(t *testing.T, conn Conn, opts ...Option) *Storage {
	t.Helper()

	s, err := New(conn, append([]Option{WithSchemaMigration(SchemaMigrationModeAdditive)}, opts...)...)
	require.NoError(t, err)

	return s
}

func TestMigrateSchemaAddsMissingColumnAndIndex(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplacingMergeTree", allColumnNames()[1:], allIndexNames()[1:])
	s := newMigratingStorage(t, conn)

	require.NoError(t, s.MigrateSchema(t.Context()))

	require.Len(t, conn.execQueries, 2)

	// Additive only, and idempotent so that replicas starting together do
	// not fight over the same statement.
	require.Contains(t, conn.execQueries[0], "ADD COLUMN IF NOT EXISTS `"+schemaColumns[0].name+"` "+schemaColumns[0].chType)
	require.Contains(t, conn.execQueries[1], "ADD INDEX IF NOT EXISTS "+skipIndexes[0].name)
	require.Contains(t, conn.execQueries[1], "GRANULARITY")

	for _, stmt := range conn.execQueries {
		require.NotContains(t, stmt, "MATERIALIZE", "materializing rewrites parts and is never automatic")
		require.NotContains(t, stmt, "DROP")
		require.NotContains(t, stmt, "MODIFY")
	}
}

func TestMigrateSchemaNoOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		conn *mockConn
		opts []Option
	}{
		{
			name: "mode off",
			conn: migrateConn("ReplacingMergeTree", nil, nil),
			opts: []Option{WithSchemaMigration(SchemaMigrationModeOff)},
		},
		{
			name: "schema already matches",
			conn: migrateConn("ReplacingMergeTree", allColumnNames(), allIndexNames()),
		},
		{
			// An absent table is not drift to patch: creating it is
			// SchemaDDL's job and the caller's decision.
			name: "table absent",
			conn: migrateConn("", nil, nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newMigratingStorage(t, tt.conn, tt.opts...)

			require.NoError(t, s.MigrateSchema(t.Context()))
			require.Empty(t, tt.conn.execQueries)
		})
	}
}

// A column on one replica and not on the others is worse than the drift it
// would be fixing, so the migration refuses rather than guesses.
func TestMigrateSchemaRefusesReplicatedWithoutCluster(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplicatedReplacingMergeTree", allColumnNames()[1:], allIndexNames())
	s := newMigratingStorage(t, conn)

	err := s.MigrateSchema(t.Context())

	require.ErrorIs(t, err, ErrUnsafeMigration)
	require.Empty(t, conn.execQueries, "nothing may be applied once the refusal is decided")
}

func TestMigrateSchemaDistributesAcrossCluster(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplicatedReplacingMergeTree", allColumnNames()[1:], allIndexNames())
	s := newMigratingStorage(t, conn, WithCluster("prod"), WithTableName("events"))

	require.NoError(t, s.MigrateSchema(t.Context()))

	require.Len(t, conn.execQueries, 1)
	require.Contains(t, conn.execQueries[0], "ALTER TABLE `events` ON CLUSTER `prod`")
}

func TestMigrateSchemaPropagatesFailure(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplacingMergeTree", allColumnNames()[1:], allIndexNames())
	conn.execFn = func(context.Context, string, ...any) error { return errBackend }

	s := newMigratingStorage(t, conn)

	require.ErrorIs(t, s.MigrateSchema(t.Context()), errBackend)
}

// Materializing is the operator's explicit call: it must cover every index
// and must not be reachable from the migration.
func TestMaterializeIndexes(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplacingMergeTree", allColumnNames(), allIndexNames())
	s := newMigratingStorage(t, conn, WithTableName("events"))

	require.NoError(t, s.MaterializeIndexes(t.Context()))

	require.Len(t, conn.execQueries, len(skipIndexes))
	for i, idx := range skipIndexes {
		require.Equal(t, "ALTER TABLE `events` MATERIALIZE INDEX "+idx.name, conn.execQueries[i])
	}
}

func TestMaterializeIndexesPropagatesFailure(t *testing.T) {
	t.Parallel()

	conn := migrateConn("ReplacingMergeTree", allColumnNames(), allIndexNames())
	conn.execFn = func(context.Context, string, ...any) error { return errBackend }

	s := newMigratingStorage(t, conn)

	require.ErrorIs(t, s.MaterializeIndexes(t.Context()), errBackend)
}

func TestWithSchemaMigrationIgnoresUnknownMode(t *testing.T) {
	t.Parallel()

	o := newOptions(WithSchemaMigration(SchemaMigrationModeAdditive), WithSchemaMigration("bogus"))

	require.Equal(t, SchemaMigrationModeAdditive, o.schemaMigrationMode)
}
