// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrSchemaMismatch is returned by [Storage.CheckSchema] under
// [SchemaCheckModeEnforce] when the live table lacks something this package
// expects.
var ErrSchemaMismatch = errors.New("audit/clickhouse: table schema is out of date")

// Queries against the system tables describing the live schema.
const (
	liveColumnsQuery = "SELECT name, type FROM system.columns WHERE database = currentDatabase() AND table = ?"
	liveIndexesQuery = "SELECT name, data_compressed_bytes, type_full, granularity, expr FROM system.data_skipping_indices " +
		"WHERE database = currentDatabase() AND table = ?"
	liveTableQuery = "SELECT engine, ifNull(total_rows, 0), sorting_key, partition_key FROM system.tables " +
		"WHERE database = currentDatabase() AND name = ?"
)

// schemaDiff is what the live table lacks compared to this package.
type schemaDiff struct {
	// engine is the live table's engine, empty when the table is absent.
	engine string

	missingColumns []columnDef
	missingIndexes []skipIndexDef

	// mismatches describe definitions that differ from the expected schema:
	// column types, skip index types and granularities, the sorting and the
	// partition key. Additive migration cannot repair them.
	mismatches []string

	// unmaterializedIndexes exist on the table but hold no data while the
	// table does, which is what an index added over populated parts looks
	// like until MATERIALIZE INDEX runs.
	//
	// This is a heuristic, not a fact: ClickHouse exposes no per-part flag
	// for it, and a partially materialized index already reports a non-zero
	// size. It is therefore only ever reported, never enforced.
	unmaterializedIndexes []skipIndexDef
}

// exists reports whether the table is there at all.
func (d schemaDiff) exists() bool { return d.engine != "" }

// empty reports whether the live table matches what this package expects.
func (d schemaDiff) empty() bool {
	return d.exists() && len(d.missingColumns) == 0 && len(d.missingIndexes) == 0 && len(d.mismatches) == 0
}

// describe renders the diff for a human.
func (d schemaDiff) describe() []string {
	parts := make([]string, 0, len(d.missingColumns)+len(d.missingIndexes)+len(d.mismatches))
	for _, c := range d.missingColumns {
		parts = append(parts, "column "+c.name)
	}

	for _, idx := range d.missingIndexes {
		parts = append(parts, "index "+idx.name)
	}

	return append(parts, d.mismatches...)
}

// diffSchema compares the live table against the columns and skip indexes
// this package expects.
func (s *Storage) diffSchema(ctx context.Context) (schemaDiff, error) {
	var diff schemaDiff

	table, err := s.liveTable(ctx)
	if err != nil {
		return diff, coreerrs.WrapOperation(err, "read live table")
	}

	if table.engine == "" {
		return diff, nil
	}
	diff.engine = table.engine

	if got, want := normalizeExpr(table.sortingKey), normalizeExpr(orderByExpr); got != want {
		diff.mismatches = append(diff.mismatches, fmt.Sprintf("sorting key is %q, want %q", got, want))
	}
	if got, want := normalizeExpr(table.partitionKey), normalizeExpr(partitionByExpr); got != want {
		diff.mismatches = append(diff.mismatches, fmt.Sprintf("partition key is %q, want %q", got, want))
	}

	columns, err := s.liveColumns(ctx)
	if err != nil {
		return diff, coreerrs.WrapOperation(err, "read live columns")
	}

	indexes, err := s.liveIndexes(ctx)
	if err != nil {
		return diff, coreerrs.WrapOperation(err, "read live skip indexes")
	}

	for _, c := range schemaColumns {
		got, ok := columns[c.name]
		switch {
		case !ok:
			diff.missingColumns = append(diff.missingColumns, c)
		case normalizeExpr(got) != normalizeExpr(c.chType):
			diff.mismatches = append(diff.mismatches, fmt.Sprintf("column %s has type %s, want %s", c.name, got, c.chType))
		}
	}

	for _, idx := range skipIndexes {
		live, ok := indexes[idx.name]
		switch {
		case !ok:
			diff.missingIndexes = append(diff.missingIndexes, idx)
		case normalizeExpr(live.expr) != normalizeExpr(idx.column):
			diff.mismatches = append(diff.mismatches, fmt.Sprintf("index %s covers %s, want %s", idx.name, live.expr, idx.column))
		case normalizeExpr(live.indexType) != normalizeExpr(idx.indexType) || live.granularity != uint64(idx.granularity): //nolint:gosec // positive constant
			diff.mismatches = append(diff.mismatches, fmt.Sprintf("index %s is %s GRANULARITY %d, want %s GRANULARITY %d",
				idx.name, live.indexType, live.granularity, idx.indexType, idx.granularity))
		case live.bytes == 0 && table.rows > 0:
			diff.unmaterializedIndexes = append(diff.unmaterializedIndexes, idx)
		}
	}

	return diff, nil
}

// CheckSchema compares the live table against the columns and skip indexes
// this package expects, and reports the drift according to
// [SchemaCheckMode].
//
// It exists because CREATE TABLE IF NOT EXISTS is not a migration: it does
// nothing at all when the table is already there, so a table created before
// a column or an index was added to this package keeps neither, silently.
//
// The check is deliberately not run by [New]: constructing a storage stays
// free of I/O, matching the driver's own lazy connect. Call this once from
// wherever the service already tolerates a startup round trip — a readiness
// probe, a migration step, or right after wiring the storage up.
//
// It reports drift and never repairs it. See [Storage.MigrateSchema] for the
// additive repair, and [Storage.MaterializeIndexes] for the part-rewriting
// one.
func (s *Storage) CheckSchema(ctx context.Context) error {
	if s.opts.schemaCheckMode == SchemaCheckModeDisabled {
		return nil
	}

	diff, err := s.diffSchema(ctx)
	if err != nil {
		return err
	}

	if !diff.exists() {
		return s.reportDrift(ctx, []string{fmt.Sprintf("table %q does not exist", s.opts.tableName)})
	}

	// Always a warning, never an error, even under Enforce: an unmaterialized
	// index costs speed rather than correctness, and the detection is a
	// heuristic. Refusing to start a service over a guess would be wrong.
	if len(diff.unmaterializedIndexes) > 0 {
		names := make([]string, len(diff.unmaterializedIndexes))
		for i, idx := range diff.unmaterializedIndexes {
			names[i] = idx.name
		}

		s.opts.logger.WarnContext(ctx, "audit skip indexes hold no data and appear to cover no existing part",
			"table", s.opts.tableName,
			"indexes", strings.Join(names, ", "),
			"remedy", "call MaterializeIndexes to backfill them")
	}

	if diff.empty() {
		return nil
	}

	return s.reportDrift(ctx, diff.describe())
}

// liveTableInfo describes the live table; an empty engine means it does not
// exist.
type liveTableInfo struct {
	engine       string
	rows         uint64
	sortingKey   string
	partitionKey string
}

// liveTable reads the live table's engine, row count and keys from
// system.tables.
func (s *Storage) liveTable(ctx context.Context) (liveTableInfo, error) {
	var info liveTableInfo

	rows, err := s.conn.Query(ctx, liveTableQuery, s.opts.tableName)
	if err != nil {
		return info, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := rows.Scan(&info.engine, &info.rows, &info.sortingKey, &info.partitionKey); err != nil {
			return info, err
		}
	}

	return info, rows.Err()
}

// liveIndexes returns the skip indexes on the table together with how many
// compressed bytes each holds.
// liveIndex describes a live skip index.
type liveIndex struct {
	bytes       uint64
	indexType   string
	granularity uint64
	expr        string
}

func (s *Storage) liveIndexes(ctx context.Context) (map[string]liveIndex, error) {
	rows, err := s.conn.Query(ctx, liveIndexesQuery, s.opts.tableName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]liveIndex)
	for rows.Next() {
		var (
			name string
			idx  liveIndex
		)

		if err := rows.Scan(&name, &idx.bytes, &idx.indexType, &idx.granularity, &idx.expr); err != nil {
			return nil, err
		}
		out[name] = idx
	}

	return out, rows.Err()
}

// liveNames collects the name column of a system-table query into a set.
// liveColumns returns the type of every live column by name.
func (s *Storage) liveColumns(ctx context.Context) (map[string]string, error) {
	rows, err := s.conn.Query(ctx, liveColumnsQuery, s.opts.tableName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		out[name] = typ
	}

	return out, rows.Err()
}

// normalizeExpr removes the parts of an expression ClickHouse may render
// differently from the DDL: whitespace and one pair of enclosing parentheses.
func normalizeExpr(expr string) string {
	expr = strings.Join(strings.Fields(expr), "")
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		expr = expr[1 : len(expr)-1]
	}
	return expr
}

// reportDrift applies [SchemaCheckMode] to what the comparison found.
func (s *Storage) reportDrift(ctx context.Context, missing []string) error {
	if s.opts.schemaCheckMode == SchemaCheckModeEnforce {
		return fmt.Errorf("%w: %s (missing: %s)",
			ErrSchemaMismatch, s.opts.tableName, strings.Join(missing, ", "))
	}

	s.opts.logger.WarnContext(ctx, "audit table schema is out of date",
		"table", s.opts.tableName,
		"missing", strings.Join(missing, ", "),
		"remedy", "MigrateSchema adds what is missing; MaterializeIndexes backfills existing parts")

	return nil
}
