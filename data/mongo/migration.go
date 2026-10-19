// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	migrate "github.com/xakep666/mongo-migrate"
)

// Migration configuration constants
const (
	// MigrationTimeout is the timeout for database migration operations
	MigrationTimeout = 30 * time.Second
	// MigrationCollectionName is the name of the collection used to track migrations
	MigrationCollectionName = "migrations"
)

// ErrInvalidMigration is returned by [RegisterMigration] for version 0, a nil up step or a version that is already registered, and
// by [Mongo.Connect] when a version is registered both with [RegisterMigration] and with the mongo-migrate global registry.
var ErrInvalidMigration = errors.New("mongo: invalid migration")

// MigrationFunc is one step of a migration, run against the client's database.
type MigrationFunc func(ctx context.Context, db *mongo.Database) error

// Migration is a registered migration, as [RegisteredMigrations] reports it.
type Migration struct {
	// Version orders the migrations and is recorded in the migrations collection once Up succeeded.
	Version uint64
	// Description is recorded with the version.
	Description string
	// Up applies the migration.
	Up MigrationFunc
	// Down reverts it on rollback (WithRollbackMigrationsOnFailure); a no-op when registered as nil.
	Down MigrationFunc
}

// migrationRegistry holds the migrations of [RegisterMigration].
var migrationRegistry struct {
	mu         sync.Mutex
	migrations []Migration
}

// RegisterMigration registers a migration that [Mongo.Connect] applies, under the database-wide migration lock, when the database's
// recorded version is below version. Register from an init function, before any Connect.
//
// The version is explicit, unlike the mongo-migrate global registry that derives it from the caller's file name, and is stored in the
// same "migrations" collection and format ({version, description, timestamp}), so a migration applied earlier through mongo-migrate
// under the same version never runs again. Version 0 (which the collection reads as "nothing applied"), a nil up and a version already
// registered are rejected with [ErrInvalidMigration]; a nil down is a no-op that keeps the migration's result on rollback.
func RegisterMigration(version uint64, description string, up, down MigrationFunc) error {
	if version == 0 {
		return fmt.Errorf("%w: version 0 is reserved", ErrInvalidMigration)
	}
	if up == nil {
		return fmt.Errorf("%w: version %d has no up step", ErrInvalidMigration, version)
	}
	if down == nil {
		down = func(context.Context, *mongo.Database) error { return nil }
	}
	migrationRegistry.mu.Lock()
	defer migrationRegistry.mu.Unlock()
	if slices.ContainsFunc(migrationRegistry.migrations, func(m Migration) bool { return m.Version == version }) {
		return fmt.Errorf("%w: version %d is already registered", ErrInvalidMigration, version)
	}
	migrationRegistry.migrations = append(migrationRegistry.migrations,
		Migration{Version: version, Description: description, Up: up, Down: down})
	return nil
}

// MustRegisterMigration is [RegisterMigration] that panics on an invalid migration, for init functions.
func MustRegisterMigration(version uint64, description string, up, down MigrationFunc) {
	if err := RegisterMigration(version, description, up, down); err != nil {
		panic(err)
	}
}

// RegisteredMigrations returns the migrations registered with [RegisterMigration], ordered by version.
func RegisteredMigrations() []Migration {
	migrationRegistry.mu.Lock()
	defer migrationRegistry.mu.Unlock()
	out := slices.Clone(migrationRegistry.migrations)
	slices.SortFunc(out, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	return out
}

// pendingMigrations merges the migrations of [RegisterMigration] with those of the mongo-migrate global registry.
func pendingMigrations() ([]migrate.Migration, error) {
	return mergeMigrations(migrate.RegisteredMigrations(), RegisteredMigrations())
}

// mergeMigrations appends typed to global. A version present in both is an [ErrInvalidMigration]: running either one would silently
// skip the other.
func mergeMigrations(global []migrate.Migration, typed []Migration) ([]migrate.Migration, error) {
	out := make([]migrate.Migration, 0, len(global)+len(typed))
	out = append(out, global...)
	for _, m := range typed {
		if slices.ContainsFunc(global, func(g migrate.Migration) bool { return g.Version == m.Version }) {
			return nil, fmt.Errorf("%w: version %d is registered with both RegisterMigration and mongo-migrate", ErrInvalidMigration, m.Version)
		}
		out = append(out, migrate.Migration{
			Version: m.Version, Description: m.Description,
			Up: migrate.MigrationFunc(m.Up), Down: migrate.MigrationFunc(m.Down),
		})
	}
	return out, nil
}

// migrate runs database migrations using the configured migration collection.
// It applies all available migrations and rolls back if any migration fails.
//
// The migration process:
//   - Merges the migrations of RegisterMigration and of the mongo-migrate
//     global registry, failing on a version registered in both
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
	migrations, err := pendingMigrations()
	if err != nil {
		return err
	}
	// A runner of its own, bound to this database, over a copy of the
	// registered migrations: the package-level mongo-migrate state is one
	// database for the whole process, so clients of different databases
	// migrating concurrently would otherwise run against each other's.
	runner := migrate.NewMigrate(m.client.Database(m.DatabaseName()), migrations...)
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
