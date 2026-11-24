// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

const (
	migrationSuffix = ".up.sql"
	migrationLedger = "schema_migrations"

	lockMigrations = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`
	createSchema   = `CREATE SCHEMA IF NOT EXISTS %s`
	createLedger   = `
		CREATE TABLE IF NOT EXISTS %s (
			version bigint PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	selectApplied = `
		SELECT EXISTS (
			SELECT 1
			  FROM %s
			 WHERE version = $1
		)`
	insertApplied = `
		INSERT INTO %s (version)
		VALUES ($1)`
)

type migration struct {
	version int64
	name    string
}

// Migrate applies the migrations of source that the ledger in schema does not list yet. Files are
// named <version>_<name>.up.sql and run in version order. The schema, the ledger and every pending
// migration are created in one transaction under an advisory lock, so concurrent starts apply
// each migration exactly once and a failed migration leaves nothing behind.
func Migrate(ctx context.Context, db *sql.DB, source fs.FS, schema string) error {
	if nilcheck.IsNil(source) || schema == "" || schema == "$user" || schema == "pg_temp" || len(schema) > 63 || strings.ContainsRune(schema, 0) {
		return errors.New("valid migration schema and source are required")
	}

	migrations, err := discoverMigrations(source)
	if err != nil {
		return err
	}

	quotedSchema := pgx.Identifier{schema}.Sanitize()
	ledger := pgx.Identifier{schema, migrationLedger}.Sanitize()

	if attached(ctx) != nil {
		return errors.New("migrations require an independent transaction")
	}
	return withTransaction(ctx, db, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context) error {
		q, _ := Tx(ctx)
		// Startup may have a longer budget than ordinary lock waits. Bound both
		// the advisory lock and the DDL by the caller's remaining migration time.
		if deadline, ok := ctx.Deadline(); ok {
			remaining := min(max(time.Until(deadline), time.Millisecond), math.MaxInt32*time.Millisecond)
			_, err := WithTransactionSettings(ctx, db, TransactionSettings{StatementTimeout: remaining, LockTimeout: remaining})
			if err != nil {
				return coreerrs.WrapOperation(err, "set migration timeouts")
			}
		}
		if _, err := q.ExecContext(ctx, lockMigrations, "go-atlas/migrations/"+schema); err != nil {
			return coreerrs.WrapOperation(err, "lock migrations")
		}

		if _, err := q.ExecContext(ctx, fmt.Sprintf(createSchema, quotedSchema)); err != nil {
			return fmt.Errorf("failed to create schema %q: %w", schema, err)
		}

		if _, err := q.ExecContext(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
			return coreerrs.WrapOperation(err, "set migration schema")
		}
		if _, err := q.ExecContext(ctx, fmt.Sprintf(createLedger, ledger)); err != nil {
			return coreerrs.WrapOperation(err, "create migration ledger")
		}

		for _, pending := range migrations {
			if err := apply(ctx, q, source, ledger, pending); err != nil {
				return fmt.Errorf("failed to apply %q migration %d: %w", schema, pending.version, err)
			}
		}

		return nil
	})
}

func apply(ctx context.Context, q *sql.Tx, source fs.FS, ledger string, pending migration) error {
	var applied bool

	err := q.QueryRowContext(ctx, fmt.Sprintf(selectApplied, ledger), pending.version).Scan(&applied)
	if err != nil {
		return err
	}

	if applied {
		return nil
	}

	ddl, err := fs.ReadFile(source, pending.name)
	if err != nil {
		return err
	}

	if _, err = q.ExecContext(ctx, string(ddl)); err != nil {
		return err
	}

	_, err = q.ExecContext(ctx, fmt.Sprintf(insertApplied, ledger), pending.version)

	return err
}

func discoverMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read migrations")
	}

	result := make([]migration, 0, len(entries))
	seen := make(map[int64]bool, len(entries))

	for _, entry := range entries {
		stem, ok := strings.CutSuffix(entry.Name(), migrationSuffix)
		prefix, name, hasName := strings.Cut(stem, "_")
		version, parseErr := strconv.ParseInt(prefix, 10, 64)

		if entry.IsDir() || !ok || !hasName || name == "" || parseErr != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		if seen[version] {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}

		seen[version] = true
		result = append(result, migration{version: version, name: entry.Name()})
	}

	slices.SortFunc(result, func(left, right migration) int {
		return cmp.Compare(left.version, right.version)
	})

	return result, nil
}
