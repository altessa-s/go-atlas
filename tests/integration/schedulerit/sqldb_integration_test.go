// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
	"github.com/altessa-s/go-atlas/service/scheduler/storagetest"
)

// envOr returns the environment override for a connection setting, or the
// default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// sqlTarget is one database/sql server of the docker stack.
type sqlTarget struct {
	driver, dsn string
	dialect     sqldb.Dialect
}

func sqlBackend(name, driver, dsn string, dialect sqldb.Dialect) backend {
	target := &sqlTarget{driver: driver, dsn: dsn, dialect: dialect}
	return backend{name: name, storage: target.storage, sql: target}
}

// open connects to the server and returns throwaway table names, dropped on
// cleanup. It skips when the server is unreachable.
func (s *sqlTarget) open(tb testing.TB) (db *sql.DB, tasks, history string) {
	tb.Helper()
	db, err := sql.Open(s.driver, s.dsn)
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		// The DSN is not printed: an override may carry a password.
		tb.Skipf("%s unreachable (%v) — start it with: make integration-up", s.dialect, err)
	}

	tasks, history = sqlTableName(tb, "tasks"), sqlTableName(tb, "history")
	// Registered before any DDL, so a failing setup still drops whatever
	// it created.
	tb.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+tasks)
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+history)
		_ = db.Close()
	})
	return db, tasks, history
}

// storage builds a storage over throwaway tables, created with EnsureSchema
// (twice, to prove idempotence).
func (s *sqlTarget) storage(tb testing.TB) scheduler.Storage {
	tb.Helper()
	db, tasks, history := s.open(tb)
	store, err := sqldb.New(db, s.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	return store
}

// sqlTableName returns a table name unique across tests and test processes:
// a readable prefix of the test name, a random suffix, and the kind. It is
// lower case because the cleanup DROP is unquoted and PostgreSQL folds an
// unquoted name to lower case.
func sqlTableName(tb testing.TB, kind string) string {
	tb.Helper()
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, strings.ToLower(tb.Name()))
	const maxPrefix = 24
	safe = safe[:min(len(safe), maxPrefix)]
	return "s_" + safe + "_" + strings.ToLower(rand.Text()[:8]) + "_" + kind
}

// TestStorageContract runs the shared storage contract suite on every backend,
// each contract over its own isolated storage. Identity, Pagination and History
// run on the SQL backends only: Redis and MongoDB do not satisfy them yet.
func TestStorageContract(t *testing.T) {
	t.Parallel()
	sqlOnly := map[string]func(*testing.T, scheduler.Storage){
		"Identity":   storagetest.Identity,
		"Pagination": storagetest.Pagination,
		"History":    storagetest.History,
	}
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, b.storage)
			if b.sql == nil {
				return
			}
			for name, check := range sqlOnly {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					check(t, b.storage(t))
				})
			}
		})
	}
}

// TestSQLFilterSemantics pins what only the SQL backends do: filters run in the
// database, so non-ASCII predicates follow SQL character semantics — endsWith
// matches, size counts characters (the memory evaluator counts bytes, a
// documented divergence) — and unbounded error strings round-trip.
func TestSQLFilterSemantics(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		if b.sql == nil {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			store := b.storage(t)
			ctx := t.Context()

			require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
				ID: "cafe", Status: scheduler.TaskStatusActive, Description: "café",
			}}))
			for expr, want := range map[string]int{
				`description.endsWith("é")`: 1,
				`description.size() == 4`:   1,
				`description.matches("^C")`: 0,
			} {
				page, err := store.TasksPaginated(ctx, scheduler.Pagination{Limit: 10}, parse(t, expr))
				require.NoError(t, err, expr)
				require.Len(t, page, want, expr)
			}

			// The SQL backend returns Tasks in byte-wise ID order (the contract
			// leaves the order to the implementation).
			for _, id := range []string{"b", "B", "a"} {
				require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: id, Status: scheduler.TaskStatusActive}}))
			}
			var order []string
			for st, err := range store.Tasks(ctx) {
				require.NoError(t, err)
				order = append(order, st.ID)
			}
			require.Equal(t, []string{"B", "a", "b", "cafe"}, order)

			// IDs up to the limit fit the columns, counted in characters.
			longID := strings.Repeat("é", sqldb.MaxIDLength)
			require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: longID, Status: scheduler.TaskStatusActive}}))
			got, err := store.GetTask(ctx, longID)
			require.NoError(t, err)
			require.Equal(t, longID, got.ID)
			require.ErrorIs(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{ID: longID + " "}}), sqldb.ErrValueTooLong)

			big := strings.Repeat("x", 70*1024)
			require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "big", TaskID: "cafe", StartedAt: 1, EndedAt: 2, Error: big}))
			for h, err := range store.History(ctx, "cafe") {
				require.NoError(t, err)
				require.Equal(t, big, h.Error)
			}
		})
	}
}

// TestSQLSchemaUpgrade pins that EnsureSchema brings a tasks table created
// before the run-lease and occurrence columns existed up to date, keeping its
// rows, and that a run on the upgraded table is claimed, renewed and finished.
func TestSQLSchemaUpgrade(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		if b.sql == nil {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			db, tasks, history := b.sql.open(t)
			store, err := sqldb.New(db, b.sql.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(ctx))
			// Reduce the table to the previous release's shape and give it a row.
			_, err = db.ExecContext(ctx, "ALTER TABLE "+tasks+" DROP COLUMN run_lease_until, DROP COLUMN run_lease_id, DROP COLUMN run_at")
			require.NoError(t, err)
			params := "?, ?"
			if b.sql.dialect == sqldb.DialectPostgres {
				params = "$1, $2"
			}
			_, err = db.ExecContext(ctx, "INSERT INTO "+tasks+" (id, status, description, schedule, next_run_at, meta) VALUES ("+params+
				", '', '@every 1m', 300, '{}')", "legacy", int32(scheduler.TaskStatusActive))
			require.NoError(t, err)

			// Instances starting together upgrade the same table concurrently.
			var wg sync.WaitGroup
			errs := make([]error, 4)
			for i := range errs {
				wg.Go(func() { errs[i] = store.EnsureSchema(ctx) })
			}
			wg.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			require.NoError(t, store.EnsureSchema(ctx), "the upgrade must be idempotent")

			got, err := store.GetTask(ctx, "legacy")
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Zero(t, got.RunLeaseUntil)
			require.Empty(t, got.RunLeaseID)
			require.Zero(t, got.RunAt)
			claimed, err := store.ClaimRun(ctx, "legacy", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: "owner/run", LeaseUntil: 400})
			require.NoError(t, err)
			require.True(t, claimed)
			renewed, err := store.RenewRun(ctx, "legacy", "owner/run", 500)
			require.NoError(t, err)
			require.True(t, renewed)
			finished, err := store.FinishRun(ctx, "legacy", "owner/run",
				scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 360, Schedule: "@every 1m", Success: true})
			require.NoError(t, err)
			require.True(t, finished)
			got, err = store.GetTask(ctx, "legacy")
			require.NoError(t, err)
			require.Equal(t, int64(360), got.NextRunAt)
			require.Zero(t, got.RunLeaseUntil, "finishing clears the lease")
		})
	}
}

func parse(t *testing.T, expr string) filter.Node {
	t.Helper()
	p, err := filter.NewParser()
	require.NoError(t, err)
	n, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	return n
}
