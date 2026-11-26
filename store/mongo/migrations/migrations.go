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
	// CleanupOnFailure compensates for partial changes when Up returns an error. It is optional.
	CleanupOnFailure MigrationFunc
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
// Use [WithCleanupOnFailure] to compensate for partial changes when this step's up returns an error.
func RegisterMigration(version uint64, description string, up, down MigrationFunc, opts ...MigrationOption) error {
	return migrationRegistry.Register(version, description, up, down, opts...)
}

// Register adds a version to this registry under the same rules as [RegisterMigration].
func (r *Registry) Register(version uint64, description string, up, down MigrationFunc, opts ...MigrationOption) error {
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
	cleanup := newMigrationOptions(opts...).cleanupOnFailure
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.migrations, func(m Migration) bool { return m.Version == version }) {
		return fmt.Errorf("%w: version %d is already registered", ErrInvalidMigration, version)
	}
	r.migrations = append(r.migrations,
		Migration{Version: version, Description: description, Up: up, Down: down, CleanupOnFailure: cleanup})
	return nil
}

// MustRegisterMigration is [RegisterMigration] that panics on an invalid migration, for init functions.
func MustRegisterMigration(version uint64, description string, up, down MigrationFunc, opts ...MigrationOption) {
	if err := RegisterMigration(version, description, up, down, opts...); err != nil {
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
func (o *options) pendingMigrations() ([]Migration, error) {
	if o.registry != nil {
		return mergeMigrations(nil, o.registry.Registered())
	}
	return mergeMigrations(migrate.RegisteredMigrations(), RegisteredMigrations())
}

// mergeMigrations appends typed to global. A version present in both is an [ErrInvalidMigration]: running either one would silently
// skip the other.
func mergeMigrations(global []migrate.Migration, typed []Migration) ([]Migration, error) {
	out := make([]Migration, 0, len(global)+len(typed))
	for _, m := range global {
		out = append(out, Migration{
			Version: m.Version, Description: m.Description,
			Up: MigrationFunc(m.Up), Down: MigrationFunc(m.Down),
		})
	}
	for _, m := range typed {
		if slices.ContainsFunc(global, func(g migrate.Migration) bool { return g.Version == m.Version }) {
			return nil, fmt.Errorf("%w: version %d is registered with both RegisterMigration and mongo-migrate", ErrInvalidMigration, m.Version)
		}
	}
	return append(out, typed...), nil
}

// Run applies registered migrations to db under the database-wide lease.
// Without [WithRegistry], it merges the default and legacy registries and rejects
// duplicate versions. Earlier successful steps remain applied on failure.
// An up error invokes only the failed step's [WithCleanupOnFailure] handler, if registered.
// Cleanup runs before any rollback, with a fresh timeout and verified ownership of the lease.
// The result preserves the up error and any cleanup error. A failed up is never recorded in the ledger.
// WithRollbackOnFailure opts into rolling back the steps applied by this run, newest first, only after the ledger version is
// reconciled with the steps this run recorded, with the lease verified, and with every revert and ledger write bound to the lease;
// steps applied by earlier runs are never touched, the failed step's own down is not invoked, and rollback stops at a migration
// without a down step with [ErrIrreversibleMigration]. The result then joins the original failure with the rollback error.
// Waiting and work have separate budgets, both bounded by ctx. The lease is renewed until the work returns, so a step that ignores
// its context never overlaps a successor; a step that returns after the work budget is still recorded in the ledger, under the lease.
func Run(ctx context.Context, db *mongo.Database, opts ...Option) error {
	o := newOptions(opts...)
	migrations, err := o.pendingMigrations()
	if err != nil {
		return err
	}
	// Use a ledger bound to this database so concurrent runs on different
	// databases cannot share mongo-migrate's package-level state.
	runner := migrate.NewMigrate(db)
	runner.SetMigrationsCollection(MigrationCollectionName)

	return withMigrationLock(ctx, db, o.logger, func(l *databaselock.Lease) error { //nolint:contextcheck // the lease carries the work and scope contexts
		start, startDescription, err := runner.Version(l.Work())
		if err != nil {
			return err
		}
		pending := slices.DeleteFunc(migrations, func(m Migration) bool {
			return m.Version <= start || m.Up == nil
		})
		slices.SortFunc(pending, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })

		r := run{db: db, runner: runner, lease: l, logger: o.logger, start: start, startDescription: startDescription}
		for _, m := range pending {
			if err := r.apply(m); err != nil {
				if !o.rollbackOnFailure {
					return err
				}
				return errors.Join(err, r.rollback())
			}
		}
		return nil
	})
}

// run is the state of one migration run under a lease.
type run struct {
	db               *mongo.Database
	runner           *migrate.Migrate
	lease            *databaselock.Lease
	logger           *slog.Logger
	start            uint64
	startDescription string
	// applied holds the steps this run applied and recorded, oldest first.
	applied []Migration
}

// apply runs one step on the work context, then records it in the ledger on the lease scope: a step that returned after the work
// budget ended is recorded, not replayed by the next run, and the record never outlives the lease.
func (r *run) apply(m Migration) error {
	if err := r.lease.Work().Err(); err != nil {
		return err
	}
	if err := m.Up(r.lease.Work(), r.db); err != nil {
		if m.CleanupOnFailure != nil {
			return errors.Join(err, r.cleanupFailedStep(m))
		}
		return err
	}
	if !r.lease.Held() {
		return databaselock.ErrLeaseLost
	}
	if err := r.runner.SetVersion(r.lease.Scope(), m.Version, m.Description); err != nil {
		// The insert may have been committed without its acknowledgement: the ledger decides.
		version, _, readErr := r.runner.Version(r.lease.Scope())
		if readErr != nil {
			return fmt.Errorf("record version %d: %w", m.Version, errors.Join(err, readErr))
		}
		if version != m.Version {
			return fmt.Errorf("record version %d: %w", m.Version, err)
		}
	}
	r.applied = append(r.applied, m)
	return nil
}

func (r *run) cleanupFailedStep(m Migration) error {
	ctx, cancel := context.WithTimeout(r.lease.Scope(), MigrationTimeout)
	defer cancel()
	if err := r.lease.Verify(ctx); err != nil {
		return fmt.Errorf("cleanup skipped for version %d, lease unverified: %w", m.Version, err)
	}
	if err := m.CleanupOnFailure(ctx, r.db); err != nil {
		return fmt.Errorf("cleanup version %d: %w", m.Version, err)
	}
	return nil
}

// current is the version the ledger should hold after the steps recorded so far.
func (r *run) current() (uint64, string) {
	if len(r.applied) == 0 {
		return r.start, r.startDescription
	}
	last := r.applied[len(r.applied)-1]
	return last.Version, last.Description
}

// rollback reverts the steps this run applied, newest first. It runs only when ownership is verified with the server and the ledger
// holds exactly the version this run recorded last, so an ambiguous write or a foreign record stops it before any down. Every revert
// runs on the lease scope (canceled the moment the lease is lost) under a fresh bounded context, and the lease is re-checked before
// every ledger write.
func (r *run) rollback() error {
	if len(r.applied) == 0 {
		return nil
	}
	scope, cancel := context.WithTimeout(r.lease.Scope(), MigrationTimeout)
	defer cancel()
	if err := r.lease.Verify(scope); err != nil {
		r.warn("migration rollback skipped: lease not verified", err)
		return fmt.Errorf("rollback skipped: %w", databaselock.ErrLeaseLost)
	}
	want, _ := r.current()
	version, _, err := r.runner.Version(scope)
	if err != nil {
		return fmt.Errorf("rollback skipped, ledger version unknown: %w", err)
	}
	if version != want {
		return fmt.Errorf("rollback skipped: ledger version is %d, this run recorded %d", version, want)
	}
	for i, m := range slices.Backward(r.applied) {
		if m.Down == nil {
			return fmt.Errorf("%w: version %d (%s)", ErrIrreversibleMigration, m.Version, m.Description)
		}
		if !r.lease.Held() {
			return databaselock.ErrLeaseLost
		}
		if err := m.Down(scope, r.db); err != nil {
			return fmt.Errorf("rollback version %d: %w", m.Version, err)
		}
		prevVersion, prevDescription := r.start, r.startDescription
		if i > 0 {
			prevVersion, prevDescription = r.applied[i-1].Version, r.applied[i-1].Description
		}
		if !r.lease.Held() {
			return databaselock.ErrLeaseLost
		}
		if err := r.runner.SetVersion(scope, prevVersion, prevDescription); err != nil {
			return fmt.Errorf("record version %d after rollback: %w", prevVersion, err)
		}
	}
	return nil
}

func (r *run) warn(msg string, err error) {
	if r.logger != nil {
		r.logger.Warn(msg, "error", err)
	}
}

// LockID identifies the database-wide migration lease.
const LockID = "migrations"

// LockWaitTimeout bounds waiting independently of the migration work budget.
const LockWaitTimeout = 30 * time.Second

func withMigrationLock(ctx context.Context, db *mongo.Database, logger *slog.Logger, fn func(*databaselock.Lease) error) error {
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
