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

// ErrIrreversibleMigration is returned (joined with the original failure) when automatic rollback reaches a migration registered
// without a down step: rollback stops there and the step stays applied.
var ErrIrreversibleMigration = errors.New("mongo: irreversible migration")

// ErrLeaseLost is returned (joined with the work's own result) when the migration lease could not be renewed: the work was canceled and
// no rollback ran.
var ErrLeaseLost = databaselock.ErrLeaseLost

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
	// Down reverts it on rollback (WithRollbackOnFailure); registered as nil, the migration is irreversible and rollback stops there
	// with [ErrIrreversibleMigration].
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
// registered are rejected with [ErrInvalidMigration]; a nil down makes the migration irreversible: rollback stops at it with
// [ErrIrreversibleMigration] and keeps its result.
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
		down = func(context.Context, *mongo.Database) error {
			return fmt.Errorf("%w: version %d (%s)", ErrIrreversibleMigration, version, description)
		}
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
// duplicate versions. Earlier successful steps remain applied on failure.
// WithRollbackOnFailure opts into rolling back the steps applied by this run, newest first, on a fresh bounded context and only
// while the lease is still held; steps applied by earlier runs are never touched, the failed step's own down is not invoked, and
// rollback stops at a migration without a down step with [ErrIrreversibleMigration]. The result then joins the original failure
// with the rollback error.
// Waiting and work have separate budgets, both bounded by ctx. The lease is renewed until the work returns.
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

	return withMigrationLock(ctx, db, o.logger, func(work context.Context, held func() bool) error {
		start, startDescription, err := runner.Version(work)
		if err != nil {
			return err
		}
		pending := slices.DeleteFunc(slices.Clone(migrations), func(m migrate.Migration) bool {
			return m.Version <= start || m.Up == nil
		})
		slices.SortFunc(pending, func(a, b migrate.Migration) int { return cmp.Compare(a.Version, b.Version) })

		var applied []migrate.Migration
		for _, m := range pending {
			// Up(1) applies the next pending step and records its version: a nil error means the ledger confirmed it.
			if err := runner.Up(work, 1); err != nil {
				if !o.rollbackOnFailure {
					return err
				}
				return errors.Join(err, rollback(ctx, db, runner, applied, start, startDescription, held, o.logger))
			}
			applied = append(applied, m)
		}
		return nil
	})
}

// rollback reverts applied, the ledger-confirmed steps of this run, newest first, on a fresh bounded context. It does nothing once
// the lease is lost. The ledger version is reset to the step below each reverted one, ending at the version the run started from.
func rollback(
	ctx context.Context, db *mongo.Database, runner *migrate.Migrate, applied []migrate.Migration,
	start uint64, startDescription string, held func() bool, logger *slog.Logger,
) error {
	if len(applied) == 0 {
		return nil
	}
	if !held() {
		if logger != nil {
			logger.Warn("migration rollback skipped: lease lost")
		}
		return nil
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), MigrationTimeout)
	defer cancel()
	for i, m := range slices.Backward(applied) {
		if m.Down == nil {
			return fmt.Errorf("%w: version %d (%s)", ErrIrreversibleMigration, m.Version, m.Description)
		}
		if !held() {
			return databaselock.ErrLeaseLost
		}
		if err := m.Down(rollbackCtx, db); err != nil {
			return fmt.Errorf("rollback version %d: %w", m.Version, err)
		}
		prevVersion, prevDescription := start, startDescription
		if i > 0 {
			prevVersion, prevDescription = applied[i-1].Version, applied[i-1].Description
		}
		if err := runner.SetVersion(rollbackCtx, prevVersion, prevDescription); err != nil {
			return fmt.Errorf("record version %d after rollback: %w", prevVersion, err)
		}
	}
	return nil
}

// LockID identifies the database-wide migration lease.
const LockID = "migrations"

// LockWaitTimeout bounds waiting independently of the migration work budget.
const LockWaitTimeout = 30 * time.Second

func withMigrationLock(ctx context.Context, db *mongo.Database, logger *slog.Logger, fn func(context.Context, func() bool) error) error {
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
	return databaselock.Hold(ctx, collection, LockID, owner, MigrationTimeout, MigrationTimeout, fn)
}
