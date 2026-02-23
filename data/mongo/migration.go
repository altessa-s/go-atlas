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
//   - Sets the database and migration collection
//   - Runs all available migrations with UP direction
//   - If any migration fails, attempts to rollback all migrations
//   - Logs warnings for rollback failures but returns the original error
//
// Parameters:
//   - ctx: Context for controlling migration timeout and cancellation
//
// Returns:
//   - error: Error if migrations fail or if rollback is needed
func (m *Mongo) migrate(ctx context.Context) error {
	migrate.SetDatabase(m.client.Database(m.DatabaseName()))
	migrate.SetMigrationsCollection(MigrationCollectionName)

	if err := migrate.Up(ctx, migrate.AllAvailable); err != nil {
		if rollbackErr := migrate.Down(ctx, migrate.AllAvailable); rollbackErr != nil {
			if m.config.Logger != nil {
				m.config.Logger.Warn("failed to rollback migrations", slog.Any("error", err), "rollback_error", rollbackErr)
			}
		}
		return err
	}

	return nil
}
