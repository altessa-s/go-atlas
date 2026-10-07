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

// ErrUnsafeMigration is returned by [Storage.MigrateSchema] when applying
// the change would leave replicas disagreeing about the schema.
var ErrUnsafeMigration = errors.New("audit/clickhouse: refusing to migrate a replicated table without a cluster")

// replicatedEnginePrefix marks the engine family whose DDL must be
// distributed with ON CLUSTER.
const replicatedEnginePrefix = "Replicated"

// MigrateSchema adds the columns and skip indexes the live table is missing.
//
// Only additive, metadata-only statements are issued — ADD COLUMN and
// ADD INDEX, both with IF NOT EXISTS so that several replicas starting at
// once do not fight. Neither touches a single existing part: the table
// definition changes, and the data stays where it is. Nothing is ever
// modified or dropped, so this cannot lose data and has no down direction.
//
// A newly added index only covers parts written from now on. Backfilling it
// over existing parts is [Storage.MaterializeIndexes], which is a different
// kind of operation entirely and is never done here.
//
// On a Replicated* table the statements must reach every replica, so a
// cluster is required: without one this returns [ErrUnsafeMigration] rather
// than creating a table that differs between replicas, which is worse than
// the drift it would be fixing.
//
// A no-op when [SchemaMigrationModeOff] is configured.
func (s *Storage) MigrateSchema(ctx context.Context) error {
	if s.opts.schemaMigrationMode == SchemaMigrationModeOff {
		return nil
	}

	diff, err := s.diffSchema(ctx)
	if err != nil {
		return err
	}

	// An absent table is not drift to patch: it is a table to create, which
	// is SchemaDDL's job and the caller's decision.
	if !diff.exists() || diff.empty() {
		return nil
	}

	if strings.HasPrefix(diff.engine, replicatedEnginePrefix) && s.opts.cluster == "" {
		return fmt.Errorf("%w: table %s uses %s", ErrUnsafeMigration, s.opts.tableName, diff.engine)
	}

	for _, c := range diff.missingColumns {
		stmt := fmt.Sprintf("%s ADD COLUMN IF NOT EXISTS `%s` %s", s.alterPrefix(), c.name, c.chType)
		if err := s.applyMigration(ctx, stmt, "column", c.name); err != nil {
			return err
		}
	}

	for _, idx := range diff.missingIndexes {
		stmt := fmt.Sprintf("%s ADD INDEX IF NOT EXISTS %s `%s` TYPE %s GRANULARITY %d",
			s.alterPrefix(), idx.name, idx.column, idx.indexType, idx.granularity)
		if err := s.applyMigration(ctx, stmt, "index", idx.name); err != nil {
			return err
		}
	}

	if len(diff.missingIndexes) > 0 {
		s.opts.logger.InfoContext(ctx, "added skip indexes cover new parts only",
			"table", s.opts.tableName,
			"remedy", "call MaterializeIndexes to backfill existing parts")
	}

	return nil
}

// MaterializeIndexes backfills every skip index over the parts already on
// disk.
//
// This is a mutation, not a metadata change: ClickHouse rewrites every
// affected part in the background, and the cost scales with the table. On a
// busy audit table that competes for disk with the very inserts the trail
// depends on, which is why nothing calls this automatically — not [New], not
// the factory, not [Storage.MigrateSchema]. It is an operator's decision.
//
// The statements are issued and the call returns; the work continues in the
// background. Watch system.mutations for progress.
func (s *Storage) MaterializeIndexes(ctx context.Context) error {
	for _, idx := range skipIndexes {
		stmt := fmt.Sprintf("%s MATERIALIZE INDEX %s", s.alterPrefix(), idx.name)
		if err := s.conn.Exec(ctx, stmt); err != nil {
			return coreerrs.WrapField(err, idx.name)
		}

		s.opts.logger.InfoContext(ctx, "materializing audit skip index",
			"table", s.opts.tableName, "index", idx.name)
	}

	return nil
}

// alterPrefix renders the ALTER TABLE head, distributed across the cluster
// when one is configured.
func (s *Storage) alterPrefix() string {
	if s.opts.cluster != "" {
		return fmt.Sprintf("ALTER TABLE `%s` ON CLUSTER `%s`", s.opts.tableName, s.opts.cluster)
	}

	return fmt.Sprintf("ALTER TABLE `%s`", s.opts.tableName)
}

// applyMigration runs one additive statement and records what it did.
func (s *Storage) applyMigration(ctx context.Context, stmt, kind, name string) error {
	if err := s.conn.Exec(ctx, stmt); err != nil {
		return coreerrs.WrapOperation(err, "add audit "+kind+" "+name)
	}

	s.opts.logger.InfoContext(ctx, "migrated audit table",
		"table", s.opts.tableName, kind, name)

	return nil
}
