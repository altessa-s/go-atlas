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

// sqlStorage builds a storage over throwaway tables, created with EnsureSchema
// (twice, to prove idempotence) and dropped on cleanup. It skips when the
// server is unreachable.
func sqlStorage(driver, dsn string, dialect sqldb.Dialect) func(tb testing.TB) scheduler.Storage {
	return func(tb testing.TB) scheduler.Storage {
		tb.Helper()
		db, err := sql.Open(driver, dsn)
		require.NoError(tb, err)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			// The DSN is not printed: an override may carry a password.
			tb.Skipf("%s unreachable (%v) — start it with: make integration-up", dialect, err)
		}

		tasks, history := sqlTableName(tb, "tasks"), sqlTableName(tb, "history")
		// Registered before any DDL, so a failing setup still drops whatever
		// it created.
		tb.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+tasks)
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+history)
			_ = db.Close()
		})
		store, err := sqldb.New(db, dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
		require.NoError(tb, err)
		require.NoError(tb, store.EnsureSchema(tb.Context()))
		require.NoError(tb, store.EnsureSchema(tb.Context()))
		return store
	}
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
// each contract over its own isolated storage.
func TestStorageContract(t *testing.T) {
	t.Parallel()
	contracts := map[string]func(*testing.T, scheduler.Storage){
		"finish_run":      storagetest.FinishRun,
		"replace_task_if": storagetest.ReplaceTaskIf,
		"claim_run":       storagetest.ClaimRun,
		"due_tasks":       storagetest.DueTasks,
		"identity":        storagetest.Identity,
		"pagination":      storagetest.Pagination,
		"history":         storagetest.History,
	}
	for _, b := range backends() {
		if !strings.Contains("postgres mariadb mysql", b.name) {
			continue // Mongo and Redis keep their own suites in the root module.
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			for name, check := range contracts {
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
		if !strings.Contains("postgres mariadb mysql", b.name) {
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

func parse(t *testing.T, expr string) filter.Node {
	t.Helper()
	p, err := filter.NewParser()
	require.NoError(t, err)
	n, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	return n
}
