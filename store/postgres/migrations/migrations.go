// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/store/postgres"
)

// ErrInvalidMigration is returned by [Register] for a version below 1, a nil up step or a version that is already registered.
var ErrInvalidMigration = errors.New("postgres migrations: invalid migration")

// ErrIrreversibleMigration is returned by [Down] for a migration registered without a down step: reverting it fails, its transaction
// rolls back and its version stays recorded, so a later [Run] does not apply it again over its own result.
var ErrIrreversibleMigration = errors.New("postgres migrations: irreversible migration")

// MigrationFunc is one step of a migration, run inside the transaction that also records the version.
type MigrationFunc func(ctx context.Context, tx *sql.Tx) error

// Migration is a registered migration, as [Registered] reports it.
type Migration struct {
	// Version orders the migrations and is recorded in the version table once Up committed.
	Version int64
	// Description documents the migration; it is not stored.
	Description string
	// Up applies the migration.
	Up MigrationFunc
	// Down reverts it ([Down]); registered as nil, it fails with [ErrIrreversibleMigration].
	Down MigrationFunc
}

// MigrationStatus is the state of one registered migration in a database, as [Status] reports it.
type MigrationStatus struct {
	// Version and Description identify the migration.
	Version     int64
	Description string
	// Applied reports whether the version is recorded in the database.
	Applied bool
	// AppliedAt is when it was recorded; zero when not applied.
	AppliedAt time.Time
}

// registry holds the migrations of [Register].
var registry struct {
	mu         sync.Mutex
	migrations []Migration
}

// Register registers a migration that [Run] applies, under the advisory lock, when the database has not recorded version yet.
// Register from an init function of the package owning the tables, before any Run.
//
// Every migration runs in its own transaction together with the insert of its version, so a failed step leaves neither its changes
// nor its version behind. A version below 1, a nil up and a version already registered are rejected with [ErrInvalidMigration]; a
// nil down makes the migration irreversible: [Down] fails on it with [ErrIrreversibleMigration] and keeps its version recorded.
func Register(version int64, description string, up, down MigrationFunc) error {
	if version < 1 {
		return fmt.Errorf("%w: version %d is not positive", ErrInvalidMigration, version)
	}
	if up == nil {
		return fmt.Errorf("%w: version %d has no up step", ErrInvalidMigration, version)
	}
	if down == nil {
		down = func(context.Context, *sql.Tx) error {
			return fmt.Errorf("%w: version %d (%s)", ErrIrreversibleMigration, version, description)
		}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if slices.ContainsFunc(registry.migrations, func(m Migration) bool { return m.Version == version }) {
		return fmt.Errorf("%w: version %d is already registered", ErrInvalidMigration, version)
	}
	registry.migrations = append(registry.migrations, Migration{Version: version, Description: description, Up: up, Down: down})
	return nil
}

// MustRegister is [Register] that panics on an invalid migration, for init functions.
func MustRegister(version int64, description string, up, down MigrationFunc) {
	if err := Register(version, description, up, down); err != nil {
		panic(err)
	}
}

// Registered returns a copy of the migrations registered with [Register], ordered by version.
func Registered() []Migration {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	out := slices.Clone(registry.migrations)
	slices.SortFunc(out, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	return out
}

// Run applies every registered migration db has not recorded yet, in version order, holding a PostgreSQL session-level advisory
// lock for the whole run so replicas starting together migrate once. Waiting for the lock is bounded by the lockTimeout option; the
// migrations themselves by ctx. Every run builds its own goose.Provider over a copy of the registry: goose's global registry and
// settings are neither read nor changed. A pending migration registered below the highest applied version (out of order) fails the
// run before anything is applied. With nothing registered Run does nothing.
func Run(ctx context.Context, db *sql.DB, opts ...Option) error {
	return migrate(ctx, db, false, opts...)
}

// Down reverts the latest applied version. With nothing applied it succeeds. An irreversible version stays applied.
func Down(ctx context.Context, db *sql.DB, opts ...Option) error {
	return migrate(ctx, db, true, opts...)
}

// ErrDirectConnectionRequired indicates that the connection could not be verified as a direct PostgreSQL session.
var ErrDirectConnectionRequired = postgres.ErrDirectConnectionRequired

// ErrInvalidTableName indicates a ledger identifier incompatible with goose's unquoted SQL and case-sensitive catalog lookup.
var ErrInvalidTableName = errors.New("postgres migrations: invalid version table name")

var tableNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}(\.[a-z_][a-z0-9_]{0,62})?$`)

// PostgreSQL 17 kwlist.h classifies these words as RESERVED_KEYWORD or
// TYPE_FUNC_NAME_KEYWORD. Neither class can stand alone as a relation name.
//
//nolint:misspell // PostgreSQL recognizes both spellings of ANALYZE.
var relationKeywords = strings.Fields(`
 all analyse analyze and any array as asc asymmetric authorization binary both case cast
 check collate collation column concurrently constraint create cross current_catalog current_date
 current_role current_schema current_time current_timestamp current_user default deferrable desc
 distinct do else end except false fetch for foreign freeze from full grant group having ilike
 in initially inner intersect into is isnull join lateral leading left like limit localtime
 localtimestamp natural not notnull null offset on only or order outer overlaps placing primary
 references returning right select session_user similar some symmetric system_user table tablesample
 then to trailing true union unique user using variadic verbose when where window with`)

func settings(db *sql.DB, opts []Option) (*options, error) {
	o := newOptions(opts...)
	first, _, _ := strings.Cut(o.tableName, ".")
	if !tableNamePattern.MatchString(o.tableName) || slices.Contains(relationKeywords, first) {
		return nil, ErrInvalidTableName
	}
	if db == nil {
		return nil, errors.New("postgres migrations: nil database")
	}
	return o, nil
}

func migrate(ctx context.Context, db *sql.DB, down bool, opts ...Option) error {
	o, err := settings(db, opts)
	if err != nil {
		return err
	}
	registered := Registered()
	if len(registered) == 0 {
		return nil
	}
	op := "up"
	if down {
		op = "down"
	}
	acquire, cancel := context.WithTimeout(ctx, o.lockTimeout)
	defer cancel()
	conn, err := directConn(acquire, db)
	if err != nil {
		return diagnostic(op+" connect", err)
	}
	defer func() { _ = conn.Close() }()
	err = withLock(ctx, acquire, conn, o, func(pinned *sql.DB) error {
		steps := make([]*goose.Migration, 0, len(registered))
		for _, m := range registered {
			steps = append(steps, goose.NewGoMigration(m.Version, transactionalStep(m.Up), transactionalStep(m.Down)))
		}
		p, runErr := goose.NewProvider(goose.DialectPostgres, pinned, nil,
			goose.WithDisableGlobalRegistry(true), goose.WithGoMigrations(steps...),
			goose.WithTableName(o.tableName), goose.WithSlog(o.logger),
		)
		if runErr != nil {
			return runErr
		}
		if down {
			_, runErr = p.Down(ctx)
			if runErr == goose.ErrNoNextVersion { //nolint:errorlint // only goose’s empty result; callback failures may wrap this sentinel
				return nil
			}
		} else {
			_, runErr = p.Up(ctx)
		}
		return runErr
	})
	return diagnostic(op, err)
}

// Goose rolls back returned errors. Release the transaction on panic as well, before
// goose closes its pinned connection (which waits for the transaction to finish).
func transactionalStep(step MigrationFunc) *goose.GoFunc {
	return &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		returned := false
		defer func() {
			if !returned {
				_ = tx.Rollback()
			}
		}()
		err := step(ctx, tx)
		returned = true
		return err
	}}
}

func diagnostic(op string, err error) error {
	if err == nil {
		return nil
	}
	if failure, ok := errors.AsType[*goose.PartialError](err); ok && failure.Failed != nil && failure.Failed.Source != nil {
		op = fmt.Sprintf("%s version %d", op, failure.Failed.Source.Version)
	}
	safe := sqldialect.SafeError(nil, "postgres migrations "+op, err)
	for _, candidate := range []error{ErrIrreversibleMigration, ErrInvalidMigration, ErrDirectConnectionRequired, ErrInvalidTableName} {
		if errors.Is(err, candidate) {
			safe = fmt.Errorf("%w: %w", safe, candidate)
		}
	}
	return safe
}

// sqlStateUndefinedTable is the SQLSTATE of a missing table (undefined_table).
const sqlStateUndefinedTable = "42P01"

// Status reports every registered migration and whether db has recorded it, ordered by version. It reads the version table in a
// single query, so the report is one state of the database — while a run applies migrations, the applied ones are always a prefix
// of the registered ones — and it neither takes the lock nor writes (a missing version table means nothing is applied). With nothing
// registered it returns nil.
func Status(ctx context.Context, db *sql.DB, opts ...Option) ([]MigrationStatus, error) {
	o, err := settings(db, opts)
	if err != nil {
		return nil, err
	}
	registered := Registered()
	if len(registered) == 0 {
		return nil, nil
	}
	conn, err := directConn(ctx, db)
	if err != nil {
		return nil, diagnostic("status", err)
	}
	defer func() { _ = conn.Close() }()
	recorded, err := recordedVersions(ctx, conn, o.tableName)
	if err != nil {
		return nil, diagnostic("status", err)
	}
	out := make([]MigrationStatus, 0, len(registered))
	for _, m := range registered {
		st := MigrationStatus{Version: m.Version, Description: m.Description}
		if r, ok := recorded[m.Version]; ok && r.applied {
			st.Applied, st.AppliedAt = true, r.at
		}
		out = append(out, st)
	}
	return out, nil
}

// versionRecord is the latest row of one version in the version table.
type versionRecord struct {
	applied bool
	at      time.Time
}

// recordedVersions reads the whole version table in one query; a later row of a version overrides an earlier one, as in goose.
func recordedVersions(ctx context.Context, db *sql.Conn, table string) (map[int64]versionRecord, error) {
	// #nosec G202 -- settings validates table against tableNamePattern before Status calls recordedVersions.
	rows, err := db.QueryContext(ctx, "SELECT version_id, is_applied, tstamp FROM "+table+" ORDER BY id")
	if err != nil {
		if sqldialect.SQLState(err) == sqlStateUndefinedTable {
			return map[int64]versionRecord{}, nil // no version table: nothing applied
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]versionRecord{}
	for rows.Next() {
		var (
			version int64
			r       versionRecord
		)
		var timestamp sql.NullTime
		if err := rows.Scan(&version, &r.applied, &timestamp); err != nil {
			return nil, err
		}
		r.at = timestamp.Time
		out[version] = r
	}
	return out, rows.Err()
}
