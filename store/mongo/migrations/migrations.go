// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/internal/databaselock"

	corecontext "github.com/altessa-s/go-atlas-core/context"
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
// by [Run] when a version is registered both with [RegisterMigration] and with the mongo-migrate global registry.
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
	// Down reverts it on rollback (WithRollbackOnFailure); a no-op when registered as nil.
	Down MigrationFunc
}

var migrationRegistry Registry

// Registry holds an independent set of migrations. Its zero value is ready for use.
type Registry struct {
	mu         sync.Mutex
	migrations []Migration
}

// RegisterMigration registers a migration that [Run] applies, under the database-wide migration lock, when the database's
// recorded version is below version. Register from an init function, before any Connect.
//
// The version is explicit, unlike the mongo-migrate global registry that derives it from the caller's file name, and is stored in the
// same "migrations" collection and format ({version, description, timestamp}), so a migration applied earlier through mongo-migrate
// under the same version never runs again. Version 0 (which the collection reads as "nothing applied"), a nil up and a version already
// registered are rejected with [ErrInvalidMigration]; a nil down is a no-op that keeps the migration's result on rollback.
func RegisterMigration(version uint64, description string, up, down MigrationFunc) error {
	return migrationRegistry.Register(version, description, up, down)
}

// Register adds a version to this registry under the same rules as [RegisterMigration].
func (r *Registry) Register(version uint64, description string, up, down MigrationFunc) error {
	if version == 0 {
		return fmt.Errorf("%w: version 0 is reserved", ErrInvalidMigration)
	}
	if up == nil {
		return fmt.Errorf("%w: version %d has no up step", ErrInvalidMigration, version)
	}
	if down == nil {
		down = func(context.Context, *mongo.Database) error { return nil }
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.migrations, func(m Migration) bool { return m.Version == version }) {
		return fmt.Errorf("%w: version %d is already registered", ErrInvalidMigration, version)
	}
	r.migrations = append(r.migrations,
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
func RegisteredMigrations() []Migration { return migrationRegistry.Registered() }

// Registered returns a sorted snapshot of this registry.
func (r *Registry) Registered() []Migration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.migrations)
	slices.SortFunc(out, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	return out
}

// pendingMigrations preserves legacy init registration for the default runner.
func (o *options) pendingMigrations() ([]migrate.Migration, error) {
	if o.registry != nil {
		return mergeMigrations(nil, o.registry.Registered())
	}
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

// Run applies registered migrations to db under the database-wide lease.
// Without [WithRegistry], it merges the default and legacy registries and rejects
// duplicate versions. Earlier successful steps remain applied on
// failure. WithRollbackOnFailure opts into rolling back all applied steps.
// Waiting and work have separate budgets, both bounded by ctx.
func Run(ctx context.Context, db *mongo.Database, opts ...Option) error {
	o := newOptions(opts...)
	migrations, err := o.pendingMigrations()
	if err != nil {
		return err
	}
	// A runner of its own, bound to this database, over a copy of the
	// registered migrations: the package-level mongo-migrate state is one
	// database for the whole process, so clients of different databases
	// migrating concurrently would otherwise run against each other's.
	runner := migrate.NewMigrate(db, migrations...)
	runner.SetMigrationsCollection(MigrationCollectionName)

	return withMigrationLock(ctx, db, o.logger, func(ctx context.Context) error {
		if err := runner.Up(ctx, migrate.AllAvailable); err != nil {
			if !o.rollbackOnFailure {
				return err
			}
			if rollbackErr := runner.Down(ctx, migrate.AllAvailable); rollbackErr != nil {
				if o.logger != nil {
					o.logger.Warn("failed to rollback migrations", slog.Any("error", err), "rollback_error", rollbackErr)
				}
			}
			return err
		}

		return nil
	})
}

// LockID identifies the database-wide migration lease.
const LockID = "migrations"

// LockWaitTimeout bounds waiting independently of the migration work budget.
const LockWaitTimeout = 30 * time.Second

func withMigrationLock(ctx context.Context, db *mongo.Database, logger *slog.Logger, fn func(context.Context) error) error {
	collection := db.Collection(databaselock.CollectionName)
	owner := bson.NewObjectID()
	acquireCtx, cancel := corecontext.WithMaxTimeout(ctx, LockWaitTimeout)
	defer cancel()
	if err := databaselock.Acquire(acquireCtx, collection, LockID, owner, MigrationTimeout); err != nil {
		return err
	}
	defer func() {
		if err := databaselock.Release(ctx, collection, LockID, owner); err != nil && logger != nil {
			logger.Warn("failed to release migration lock", "error", err, "collection", databaselock.CollectionName)
		}
	}()
	workCtx, workCancel := corecontext.WithMaxTimeout(ctx, MigrationTimeout)
	defer workCancel()
	return fn(workCtx)
}
