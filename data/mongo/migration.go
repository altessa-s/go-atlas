// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"log/slog"
	"time"

	migrate "github.com/xakep666/mongo-migrate"
)

// Migration configuration constants
const (
	// MigrationTimeout is the timeout for database migration operations
	MigrationTimeout = 30 * time.Second
	// MigrationCollectionName is the name of the collection used to track migrations
	MigrationCollectionName = "migrations"
)

// migrate runs database migrations using the configured migration collection.
// It applies all available migrations and rolls back if any migration fails.
//
// The migration process:
//   - Builds a runner bound to this database and its migration collection
//   - Acquires the database-wide migration lock, waiting for any peer that
//     holds it (see [Mongo.withMigrationLock] for why this is required)
//   - Runs all available migrations with UP direction
//   - If a migration fails, returns its error and keeps every migration
//     applied so far; with WithRollbackMigrationsOnFailure it first rolls back
//     all migrations, logging a rollback failure but returning the original
//     error
//   - Releases the lock
//
// Parameters:
//   - ctx: Context for controlling migration timeout and cancellation
//
// Returns:
//   - error: Error if the lock cannot be acquired, migrations fail, or if
//     rollback is needed
func (m *Mongo) migrate(ctx context.Context) error {
	// A runner of its own, bound to this database, over a copy of the
	// registered migrations: the package-level mongo-migrate state is one
	// database for the whole process, so clients of different databases
	// migrating concurrently would otherwise run against each other's.
	runner := migrate.NewMigrate(m.client.Database(m.DatabaseName()), migrate.RegisteredMigrations()...)
	runner.SetMigrationsCollection(MigrationCollectionName)

	return m.withMigrationLock(ctx, func(ctx context.Context) error {
		if err := runner.Up(ctx, migrate.AllAvailable); err != nil {
			if !m.config.RollbackMigrationsOnFailure {
				return err
			}
			if rollbackErr := runner.Down(ctx, migrate.AllAvailable); rollbackErr != nil {
				if m.config.Logger != nil {
					m.config.Logger.Warn("failed to rollback migrations", slog.Any("error", err), "rollback_error", rollbackErr)
				}
			}
			return err
		}

		return nil
	})
}
