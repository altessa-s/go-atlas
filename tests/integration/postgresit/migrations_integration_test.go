// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/store/postgres/migrations"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

// The test migrations register in the process-wide registry, as an application's do. Only the tests of this file run them, each on
// its own throwaway database.
const (
	versionCreateProbe int64 = 2026112301
	versionSeedProbe   int64 = 2026112302
)

// Only the run carrying this context value pauses inside the second migration.
type migrationGate struct {
	entered chan time.Time
	resume  chan struct{}
}

func init() {
	migrations.MustRegister(versionCreateProbe, "create probe", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE mig_probe (id int PRIMARY KEY)`)
		return err
	}, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TABLE mig_probe`)
		return err
	})
	migrations.MustRegister(versionSeedProbe, "seed probe", func(ctx context.Context, tx *sql.Tx) error {
		if gate, ok := ctx.Value(migrationGateKey{}).(*migrationGate); ok {
			deadline, _ := ctx.Deadline()
			gate.entered <- deadline
			select {
			case <-gate.resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO mig_probe (id) VALUES (1)`)
		return err
	}, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM mig_probe WHERE id = 1`)
		return err
	})
}

type migrationGateKey struct{}

func applied(t testing.TB, db *sql.DB) map[int64]bool {
	t.Helper()
	status, err := migrations.Status(t.Context(), db)
	require.NoError(t, err)
	out := map[int64]bool{}
	for _, s := range status {
		out[s.Version] = s.Applied
	}
	return out
}

func TestMigrations_RunIsIdempotentAndDownReverts(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)

	require.Equal(t, map[int64]bool{versionCreateProbe: false, versionSeedProbe: false}, applied(t, db))
	require.NoError(t, migrations.Down(t.Context(), db), "empty ledger")
	require.NoError(t, migrations.Run(t.Context(), db))
	require.NoError(t, migrations.Run(t.Context(), db), "a second run applies nothing")
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))
	require.Equal(t, map[int64]bool{versionCreateProbe: true, versionSeedProbe: true}, applied(t, db))

	status, err := migrations.Status(t.Context(), db)
	require.NoError(t, err)
	require.Equal(t, "seed probe", status[1].Description)
	require.False(t, status[1].AppliedAt.IsZero())

	require.NoError(t, migrations.Down(t.Context(), db))
	require.Zero(t, count(t, t.Context(), db, "mig_probe"))
	require.Equal(t, map[int64]bool{versionCreateProbe: true, versionSeedProbe: false}, applied(t, db))

	require.NoError(t, migrations.Run(t.Context(), db))
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))
}

// Replicas starting together migrate once: the runs serialize on the advisory lock, and the later ones find nothing pending.
// Without the lock the second CREATE TABLE or version insert would fail.
func TestMigrations_ConcurrentRunsMigrateOnce(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)

	err := concurrency.Process(t.Context(), []int{0, 1, 2, 3}, func(ctx context.Context, _ int) error {
		return migrations.Run(ctx, db)
	}, concurrency.WithConcurrency[int](4))
	require.NoError(t, err)
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))
	require.Equal(t, map[int64]bool{versionCreateProbe: true, versionSeedProbe: true}, applied(t, db))
}

// Status reads the version table once, so while a run applies the migrations one by one it reports a
// state the run passes through: the applied migrations are always a prefix of the registered ones.
func TestMigrations_StatusIsConsistentDuringARun(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)

	gate := &migrationGate{entered: make(chan time.Time, 1), resume: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.WithValue(t.Context(), migrationGateKey{}, gate), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- migrations.Run(ctx, db, migrations.WithLockTimeout(time.Second)) }()
	select {
	case deadline := <-gate.entered:
		callerDeadline, _ := ctx.Deadline()
		require.Equal(t, callerDeadline, deadline, "migration inherits the caller deadline")
	case err := <-done:
		t.Fatalf("run finished before second migration: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Observe the public Status call on a separate client connected to the same isolated database.
	observer, err := db.Conn(ctx)
	require.NoError(t, err)
	var cfg *pgx.ConnConfig
	require.NoError(t, observer.Raw(func(raw any) error {
		cfg = raw.(interface{ Conn() *pgx.Conn }).Conn().Config()
		return nil
	}))
	require.NoError(t, observer.Close())
	trace := new(migrationTrace)
	cfg.Tracer = trace
	reader := stdlib.OpenDB(*cfg)
	defer func() { _ = reader.Close() }()
	status, err := migrations.Status(ctx, reader)
	require.NoError(t, err)
	require.Len(t, status, 2)
	var reads int
	for _, query := range trace.queries {
		if strings.Contains(query, migrations.DefaultTableName) {
			reads++
		}
	}
	require.Equal(t, 1, reads, "one ledger snapshot")
	require.True(t, status[0].Applied)
	require.False(t, status[1].Applied)
	close(gate.resume)
	require.NoError(t, <-done)
	require.True(t, applied(t, db)[versionSeedProbe])
}

// A run waits for the advisory lock at most lockTimeout, then fails without applying anything.
func TestMigrations_LockTimeout(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)
	const lockID int64 = 727274

	holder, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	_, err = holder.ExecContext(t.Context(), `SELECT pg_advisory_lock($1)`, lockID)
	require.NoError(t, err)

	start := time.Now()
	err = migrations.Run(t.Context(), db, migrations.WithLockID(lockID), migrations.WithLockTimeout(time.Second))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 10*time.Second, "the wait is bounded by the lock timeout")
	require.Equal(t, map[int64]bool{versionCreateProbe: false, versionSeedProbe: false}, applied(t, db))

	// Status reads without the lock, so it answers at once while another session holds it.
	start = time.Now()
	status, err := migrations.Status(t.Context(), db, migrations.WithLockID(lockID), migrations.WithLockTimeout(time.Second))
	require.NoError(t, err)
	require.Len(t, status, 2)
	require.Less(t, time.Since(start), 500*time.Millisecond, "Status does not wait for the lock")

	_, err = holder.ExecContext(t.Context(), `SELECT pg_advisory_unlock($1)`, lockID)
	require.NoError(t, err)
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithLockID(lockID), migrations.WithLockTimeout(time.Second)))
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))
}

// A schema-qualified version table needs its schema before goose creates it. WithCreateSchema creates it under the lock, once
// for concurrent runs, and never from Status.
func TestMigrations_CreateSchema(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)
	ledger := migrations.WithTableName("mig_ledger.versions")

	require.Error(t, migrations.Run(t.Context(), db, ledger), "goose cannot create a table in a missing schema")
	status, err := migrations.Status(t.Context(), db, ledger, migrations.WithCreateSchema())
	require.NoError(t, err)
	require.False(t, status[0].Applied)
	require.False(t, schemaExists(t, db, "mig_ledger"), "Status never creates the schema")

	err = concurrency.Process(t.Context(), []int{0, 1, 2}, func(ctx context.Context, _ int) error {
		return migrations.Run(ctx, db, ledger, migrations.WithCreateSchema())
	}, concurrency.WithConcurrency[int](3))
	require.NoError(t, err)
	require.True(t, schemaExists(t, db, "mig_ledger"))
	require.Equal(t, 3, count(t, t.Context(), db, "mig_ledger.versions"), "version zero and both migrations")
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))

	// A role without CREATE on the database can run once the schema exists.
	role := fmt.Sprintf("mig_runner_%d", time.Now().UnixNano())
	for _, stmt := range []string{
		`CREATE ROLE ` + role + ` LOGIN PASSWORD '` + role + `'`,
		`GRANT USAGE ON SCHEMA mig_ledger TO ` + role,
		`GRANT SELECT, INSERT ON mig_ledger.versions TO ` + role,
	} {
		_, err = db.ExecContext(t.Context(), stmt)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		_, _ = db.ExecContext(ctx, `DROP OWNED BY `+role)
		_, _ = db.ExecContext(ctx, `DROP ROLE `+role)
	})
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	var cfg *pgx.ConnConfig
	require.NoError(t, conn.Raw(func(raw any) error {
		cfg = raw.(interface{ Conn() *pgx.Conn }).Conn().Config().Copy()
		return nil
	}))
	require.NoError(t, conn.Close())
	cfg.User, cfg.Password = role, role
	restricted := stdlib.OpenDB(*cfg)
	defer func() { _ = restricted.Close() }()
	var canCreate bool
	require.NoError(t, restricted.QueryRowContext(t.Context(), `SELECT has_database_privilege(current_database(), 'CREATE')`).Scan(&canCreate))
	require.False(t, canCreate)
	require.NoError(t, migrations.Run(t.Context(), restricted, ledger, migrations.WithCreateSchema()))
}

// WithMigrationLockTimeout bounds each lock wait inside a migration, independently of the session's lock_timeout and of ctx.
func TestMigrations_MigrationLockTimeout(t *testing.T) {
	t.Parallel()
	db := postgrestest.New(t)

	blocker, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(t.Context(), `CREATE TABLE mig_probe (id int)`)
	require.NoError(t, err)

	start := time.Now()
	err = migrations.Run(t.Context(), db, migrations.WithMigrationLockTimeout(200*time.Millisecond))
	require.ErrorContains(t, err, "SQLSTATE 55P03")
	require.ErrorContains(t, err, "up version 2026112301")
	require.Less(t, time.Since(start), 5*time.Second, "the migration waited past its lock timeout")
	require.Equal(t, map[int64]bool{versionCreateProbe: false, versionSeedProbe: false}, applied(t, db))

	require.NoError(t, blocker.Rollback())
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithMigrationLockTimeout(200*time.Millisecond)))
	require.Equal(t, 1, count(t, t.Context(), db, "mig_probe"))
	var timeout string
	require.NoError(t, db.QueryRowContext(t.Context(), `SHOW lock_timeout`).Scan(&timeout))
	require.NotEqual(t, "200ms", timeout, "the timeout does not outlive the migration transaction")
}

func schemaExists(t testing.TB, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT to_regnamespace($1::text) IS NOT NULL`, name).Scan(&exists))
	return exists
}

// The trace records statements sent on this client, including an attempted lock that was later released.
type migrationTrace struct{ queries []string }

func (t *migrationTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.queries = append(t.queries, d.SQL)
	return ctx
}
func (*migrationTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestMigrations_RejectsPoolerBeforeLock(t *testing.T) {
	t.Parallel()
	dsn := os.Getenv("PGBOUNCER_DSN")
	if dsn == "" {
		t.Skip("PGBOUNCER_DSN not configured")
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.True(t, err == nil, "invalid PgBouncer DSN")
	trace := new(migrationTrace)
	cfg.Tracer = trace
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	require.ErrorIs(t, migrations.Run(t.Context(), db), migrations.ErrDirectConnectionRequired)
	require.Len(t, trace.queries, 1)
	require.Contains(t, strings.ToLower(trace.queries[0]), "pg_backend_pid()")
}
