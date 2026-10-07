// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"
)

// mockNameRows replays rows of the system-table queries. Each row is a slice
// of values matching the query's select list.
type mockNameRows struct {
	driver.Rows

	rows [][]any
	pos  int
	err  error
}

// nameRows builds the (name, type) rows the column query selects, with the
// type the schema expects.
func nameRows(names ...string) *mockNameRows {
	rows := make([][]any, len(names))
	for i, name := range names {
		typ := "String"
		for _, c := range schemaColumns {
			if c.name == name {
				typ = c.chType
			}
		}
		rows[i] = []any{name, typ}
	}

	return &mockNameRows{rows: rows}
}

// indexRow is the (name, data_compressed_bytes, type, granularity) row the
// skip-index query selects, with the definition the schema expects.
func indexRow(name string, bytes uint64) []any {
	typ, granularity, expr := "minmax", uint64(1), name
	for _, idx := range skipIndexes {
		if idx.name == name {
			typ, granularity, expr = idx.indexType, uint64(idx.granularity), idx.column //nolint:gosec // positive constant
		}
	}
	return []any{name, bytes, typ, granularity, expr}
}

// indexRows builds skip-index rows, marking every index as holding data.
func indexRows(names ...string) *mockNameRows {
	rows := make([][]any, len(names))
	for i, name := range names {
		rows[i] = indexRow(name, 1024)
	}

	return &mockNameRows{rows: rows}
}

// tableRow is the (engine, total_rows, sorting_key, partition_key) row the
// table query selects, with the keys the schema expects.
func tableRow(engine string, rows uint64) []any {
	return []any{engine, rows, orderByExpr, partitionByExpr}
}

func (r *mockNameRows) Next() bool {
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++

	return true
}

func (r *mockNameRows) Scan(dest ...any) error {
	src := r.rows[r.pos-1]
	for i := range dest {
		switch ptr := dest[i].(type) {
		case *string:
			*ptr = src[i].(string)
		case *uint64:
			*ptr = src[i].(uint64)
		default:
			return errBackend
		}
	}

	return nil
}

func (r *mockNameRows) Close() error { return nil }
func (r *mockNameRows) Err() error   { return r.err }

// schemaConn answers the system-table queries by looking at which one it was
// handed. The table reports one row so that an empty index counts as
// unmaterialized.
func schemaConn(columns []string, indexes *mockNameRows) *mockConn {
	return &mockConn{
		queryFn: func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
			switch {
			case strings.Contains(query, "system.tables"):
				if len(columns) == 0 {
					return &mockNameRows{}, nil
				}

				return &mockNameRows{rows: [][]any{tableRow("ReplacingMergeTree", 1)}}, nil
			case strings.Contains(query, "data_skipping_indices"):
				return indexes, nil
			default:
				return nameRows(columns...), nil
			}
		},
	}
}

// allColumnNames is what a table this package created reports back.
func allColumnNames() []string {
	names := make([]string, len(schemaColumns))
	for i, c := range schemaColumns {
		names[i] = c.name
	}

	return names
}

func allIndexNames() []string {
	names := make([]string, len(skipIndexes))
	for i, idx := range skipIndexes {
		names[i] = idx.name
	}

	return names
}

func newCheckedStorage(t *testing.T, conn Conn, mode SchemaCheckMode) (*Storage, *bytes.Buffer) {
	t.Helper()

	var logs bytes.Buffer

	s, err := New(conn,
		WithSchemaCheck(mode),
		WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)
	require.NoError(t, err)

	return s, &logs
}

func TestCheckSchemaAcceptsMatchingTable(t *testing.T) {
	t.Parallel()

	s, logs := newCheckedStorage(t, schemaConn(allColumnNames(), indexRows(allIndexNames()...)), SchemaCheckModeEnforce)

	require.NoError(t, s.CheckSchema(t.Context()))
	require.Empty(t, logs.String())
}

// New must stay free of I/O: the check is an explicit step.
func TestNewDoesNotCheckSchema(t *testing.T) {
	t.Parallel()

	conn := schemaConn(nil, indexRows())

	_, err := New(conn, WithSchemaCheck(SchemaCheckModeEnforce))

	require.NoError(t, err)
	require.Empty(t, conn.lastQuery, "constructing a storage must not query the server")
}

func TestCheckSchemaDetectsMissingParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		columns []string
		indexes []string
		want    string
	}{
		{
			name:    "missing column",
			columns: allColumnNames()[1:],
			indexes: allIndexNames(),
			want:    "column " + schemaColumns[0].name,
		},
		{
			name:    "missing index",
			columns: allColumnNames(),
			indexes: allIndexNames()[1:],
			want:    "index " + skipIndexes[0].name,
		},
		{
			name:    "no indexes at all",
			columns: allColumnNames(),
			want:    "index " + skipIndexes[0].name,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, _ := newCheckedStorage(t, schemaConn(tt.columns, indexRows(tt.indexes...)), SchemaCheckModeEnforce)

			err := s.CheckSchema(t.Context())

			require.ErrorIs(t, err, ErrSchemaMismatch)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// An empty system.columns means the table is absent; listing every column as
// missing would bury that fact.
func TestCheckSchemaReportsAbsentTable(t *testing.T) {
	t.Parallel()

	s, _ := newCheckedStorage(t, schemaConn(nil, indexRows()), SchemaCheckModeEnforce)

	err := s.CheckSchema(t.Context())

	require.ErrorIs(t, err, ErrSchemaMismatch)
	require.ErrorContains(t, err, "does not exist")
	require.NotContains(t, err.Error(), "column ", "the absent table is the finding, not its columns")
}

func TestCheckSchemaModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     SchemaCheckMode
		wantErr  bool
		wantLog  bool
		wantCall bool
	}{
		{name: "enforce fails", mode: SchemaCheckModeEnforce, wantErr: true, wantCall: true},
		{name: "warn logs", mode: SchemaCheckModeWarn, wantLog: true, wantCall: true},
		{name: "disabled is silent", mode: SchemaCheckModeDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := schemaConn(nil, indexRows())
			s, logs := newCheckedStorage(t, conn, tt.mode)

			err := s.CheckSchema(t.Context())

			if tt.wantErr {
				require.ErrorIs(t, err, ErrSchemaMismatch)
			} else {
				require.NoError(t, err)
			}

			if tt.wantLog {
				require.Contains(t, logs.String(), "schema is out of date")
				require.Contains(t, logs.String(), "MigrateSchema", "the warning must name the remedy")
			} else {
				require.Empty(t, logs.String())
			}

			if tt.wantCall {
				require.NotEmpty(t, conn.lastQuery)
			} else {
				require.Empty(t, conn.lastQuery, "a disabled check must not query the server")
			}
		})
	}
}

// An index that exists but holds no data over a populated table is what an
// index added after the fact looks like. It is reported, never enforced: the
// detection is a heuristic and the cost is speed, not correctness.
func TestCheckSchemaReportsUnmaterializedIndexes(t *testing.T) {
	t.Parallel()

	unmaterialized := &mockNameRows{rows: [][]any{
		indexRow(skipIndexes[0].name, 0),
		indexRow(skipIndexes[1].name, 2048),
		indexRow(skipIndexes[2].name, 2048),
	}}

	s, logs := newCheckedStorage(t, schemaConn(allColumnNames(), unmaterialized), SchemaCheckModeEnforce)

	require.NoError(t, s.CheckSchema(t.Context()), "a heuristic must not fail the check")

	require.Contains(t, logs.String(), "hold no data")
	require.Contains(t, logs.String(), skipIndexes[0].name)
	require.Contains(t, logs.String(), "MaterializeIndexes")
	require.NotContains(t, logs.String(), skipIndexes[1].name, "an index holding data is not reported")
}

// An empty table says nothing about its indexes: everything is empty there.
func TestCheckSchemaIgnoresEmptyIndexesOnEmptyTable(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryFn: func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
			switch {
			case strings.Contains(query, "system.tables"):
				return &mockNameRows{rows: [][]any{tableRow("ReplacingMergeTree", 0)}}, nil
			case strings.Contains(query, "data_skipping_indices"):
				rows := make([][]any, len(skipIndexes))
				for i, idx := range skipIndexes {
					rows[i] = indexRow(idx.name, 0)
				}

				return &mockNameRows{rows: rows}, nil
			default:
				return nameRows(allColumnNames()...), nil
			}
		},
	}

	s, logs := newCheckedStorage(t, conn, SchemaCheckModeEnforce)

	require.NoError(t, s.CheckSchema(t.Context()))
	require.Empty(t, logs.String())
}

func TestCheckSchemaPropagatesQueryFailure(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryFn: func(context.Context, string, ...any) (driver.Rows, error) { return nil, errBackend },
	}

	s, _ := newCheckedStorage(t, conn, SchemaCheckModeEnforce)

	require.ErrorIs(t, s.CheckSchema(t.Context()), errBackend)
}

func TestCheckSchemaPropagatesIterationFailure(t *testing.T) {
	t.Parallel()

	conn := &mockConn{
		queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
			return &mockNameRows{rows: [][]any{tableRow("ReplacingMergeTree", 1)}, err: errBackend}, nil
		},
	}

	s, _ := newCheckedStorage(t, conn, SchemaCheckModeEnforce)

	require.ErrorIs(t, s.CheckSchema(t.Context()), errBackend)
}

func TestWithSchemaCheckIgnoresUnknownMode(t *testing.T) {
	t.Parallel()

	o := newOptions(WithSchemaCheck(SchemaCheckModeEnforce), WithSchemaCheck("bogus"))

	require.Equal(t, SchemaCheckModeEnforce, o.schemaCheckMode)
}

// Definitions that differ from the schema are drift: column types, skip index
// types and granularities, and the sorting and partition keys.
func TestCheckSchemaReportsDefinitionMismatches(t *testing.T) {
	t.Parallel()

	mismatchConn := func(mutate func(table, columns, indexes [][]any)) *mockConn {
		return &mockConn{
			queryFn: func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
				table := [][]any{tableRow("ReplacingMergeTree", 1)}
				columns := nameRows(allColumnNames()...).rows
				indexes := indexRows(allIndexNames()...).rows
				mutate(table, columns, indexes)
				switch {
				case strings.Contains(query, "system.tables"):
					return &mockNameRows{rows: table}, nil
				case strings.Contains(query, "data_skipping_indices"):
					return &mockNameRows{rows: indexes}, nil
				default:
					return &mockNameRows{rows: columns}, nil
				}
			},
		}
	}

	for name, tc := range map[string]struct {
		mutate func(table, columns, indexes [][]any)
		want   string
	}{
		"column type":       {func(_, c, _ [][]any) { c[0][1] = "Int8" }, "has type Int8"},
		"index type":        {func(_, _, i [][]any) { i[0][2] = "minmax" }, "is minmax"},
		"index granularity": {func(_, _, i [][]any) { i[0][3] = uint64(99) }, "GRANULARITY 99"},
		"index expression":  {func(_, _, i [][]any) { i[0][4] = "request_id_other" }, "covers request_id_other"},
		"sorting key":       {func(tb, _, _ [][]any) { tb[0][2] = "timestamp" }, "sorting key"},
		"partition key":     {func(tb, _, _ [][]any) { tb[0][3] = "toDate(timestamp)" }, "partition key"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s, _ := newCheckedStorage(t, mismatchConn(tc.mutate), SchemaCheckModeEnforce)
			err := s.CheckSchema(t.Context())
			require.ErrorIs(t, err, ErrSchemaMismatch)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// ClickHouse may render an expression with other whitespace or parentheses.
func TestNormalizeExpr(t *testing.T) {
	t.Parallel()

	require.Equal(t, normalizeExpr(orderByExpr), normalizeExpr("toDate(timestamp), actor_id, timestamp, id"))
	require.Equal(t, "DateTime64(3,'UTC')", normalizeExpr("DateTime64(3, 'UTC')"))
	require.Equal(t, "toYYYYMM(timestamp)", normalizeExpr(" toYYYYMM(timestamp) "))
}
