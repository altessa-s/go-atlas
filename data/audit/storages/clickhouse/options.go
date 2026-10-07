// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"log/slog"
	"time"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

const (
	// DefaultTableName is the default table holding audit events.
	DefaultTableName = "audit_events"

	// DefaultEngine is the default table engine used by [SchemaDDL].
	// ReplacingMergeTree collapses the duplicate rows that at-least-once
	// delivery produces, keyed by the table's sorting key.
	DefaultEngine = "ReplacingMergeTree"

	// DefaultDDLTimeout bounds the CREATE TABLE statement [New] issues under
	// [WithAutoCreateTable].
	DefaultDDLTimeout = 30 * time.Second

	// DefaultMaxBatchSize is the largest number of events sent to ClickHouse
	// in a single INSERT. Larger inputs are split across several batches.
	DefaultMaxBatchSize = 10_000
)

// DefaultTimeRangeMode is the default reaction to a query that bounds no time
// range: run it, but make the full-table scan visible in the logs.
const DefaultTimeRangeMode = TimeRangeModeWarn

// TimeRangeMode controls how the storage reacts to an [audit.Query] with
// neither StartTime nor EndTime set. Such a query matches no partition and no
// primary-key prefix, so ClickHouse reads every part of the table.
type TimeRangeMode string

const (
	// TimeRangeModeEnforce rejects the query with [ErrTimeRangeRequired].
	TimeRangeModeEnforce TimeRangeMode = "enforce"

	// TimeRangeModeWarn logs a warning and runs the query anyway.
	TimeRangeModeWarn TimeRangeMode = "warn"

	// TimeRangeModeDisabled runs the query without complaining.
	TimeRangeModeDisabled TimeRangeMode = "disabled"
)

// DefaultSchemaCheckMode is the default reaction to a table that has drifted
// from the schema this package expects: report it, but let the service run.
const DefaultSchemaCheckMode = SchemaCheckModeWarn

// SchemaCheckMode controls what [New] does when the live table lacks a
// column or a skip index this package expects.
//
// The drift is real and silent otherwise: CREATE TABLE IF NOT EXISTS does
// nothing to a table that already exists, so anything added to the schema
// after the table was created never appears on it.
type SchemaCheckMode string

const (
	// SchemaCheckModeEnforce fails construction with [ErrSchemaMismatch].
	SchemaCheckModeEnforce SchemaCheckMode = "enforce"

	// SchemaCheckModeWarn logs what is missing and continues.
	SchemaCheckModeWarn SchemaCheckMode = "warn"

	// SchemaCheckModeDisabled skips the check, sparing two queries against
	// the system tables at startup.
	SchemaCheckModeDisabled SchemaCheckMode = "disabled"
)

// DefaultSchemaMigrationMode is the default: report drift, do not repair it.
// Changing a table is an act an operator opts into.
const DefaultSchemaMigrationMode = SchemaMigrationModeOff

// SchemaMigrationMode controls whether [Storage.MigrateSchema] is allowed to
// change the live table.
type SchemaMigrationMode string

const (
	// SchemaMigrationModeOff makes [Storage.MigrateSchema] a no-op.
	SchemaMigrationModeOff SchemaMigrationMode = "off"

	// SchemaMigrationModeAdditive permits the metadata-only statements that
	// cannot lose data: ADD COLUMN and ADD INDEX. Nothing is modified or
	// dropped, and no existing part is rewritten — backfilling an index
	// over old parts stays [Storage.MaterializeIndexes], by hand.
	SchemaMigrationModeAdditive SchemaMigrationMode = "additive"
)

// options carries the tunable [Storage] configuration.
type options struct {
	tableName    string        `optval:"nonempty" optgen:"default=DefaultTableName"`
	engine       string        `optval:"nonempty" optgen:"default=DefaultEngine"`
	ttl          time.Duration `optval:"positive"`
	maxBatchSize int           `optval:"positive" optgen:"default=DefaultMaxBatchSize"`
	ddlTimeout   time.Duration `opt:"DDLTimeout" optval:"positive" optgen:"default=DefaultDDLTimeout"`

	// cluster adds an ON CLUSTER clause to the statement [SchemaDDL]
	// renders. Empty means a single-node table. It is not enough on its
	// own for a replicated deployment: pair it with a Replicated* engine,
	// since ON CLUSTER only distributes the DDL, not the data.
	cluster string

	// autoCreateTable issues the [SchemaDDL] statement from New. Off by
	// default: schema changes belong in a migration, not in every process
	// that writes an event.
	autoCreateTable bool

	// final adds the FINAL modifier to reads, making them observe the
	// deduplicated view of a ReplacingMergeTree table at the cost of merging
	// parts at query time.
	final bool

	timeRangeMode       TimeRangeMode       `optgen:"manual,default=DefaultTimeRangeMode"`
	schemaCheckMode     SchemaCheckMode     `optgen:"manual,default=DefaultSchemaCheckMode"`
	schemaMigrationMode SchemaMigrationMode `optgen:"manual,default=DefaultSchemaMigrationMode"`
	logger              *slog.Logger
}

// WithSchemaMigration selects whether [Storage.MigrateSchema] may change the
// live table. Defaults to [DefaultSchemaMigrationMode]; an unknown mode
// leaves the configured one in place.
func WithSchemaMigration(mode SchemaMigrationMode) Option {
	return func(o *options) {
		switch mode {
		case SchemaMigrationModeOff, SchemaMigrationModeAdditive:
			o.schemaMigrationMode = mode
		}
	}
}

// WithSchemaCheck selects how [New] reacts to a table that has drifted from
// the schema this package expects. Defaults to [DefaultSchemaCheckMode]; an
// unknown mode leaves the configured one in place.
func WithSchemaCheck(mode SchemaCheckMode) Option {
	return func(o *options) {
		switch mode {
		case SchemaCheckModeEnforce, SchemaCheckModeWarn, SchemaCheckModeDisabled:
			o.schemaCheckMode = mode
		}
	}
}

// WithTimeRangeMode selects how the storage reacts to a query that bounds no
// time range. Defaults to [DefaultTimeRangeMode]; an unknown mode leaves the
// configured one in place.
func WithTimeRangeMode(mode TimeRangeMode) Option {
	return func(o *options) {
		switch mode {
		case TimeRangeModeEnforce, TimeRangeModeWarn, TimeRangeModeDisabled:
			o.timeRangeMode = mode
		}
	}
}
