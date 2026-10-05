// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
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
// each contract over its own isolated storage.
func TestStorageContract(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, b.storage)
		})
	}
}

// TestSQLFilterSemantics pins what only the SQL backends do: filters run in the
// database, so non-ASCII predicates follow SQL character semantics — endsWith
// matches, size counts characters — and unbounded error strings round-trip.
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
			checkTrailingSpaceFilters(t, store, b.sql.dialect)
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

// checkTrailingSpaceFilters pins the documented trailing-space behavior of
// filters on string fields: exact on PostgreSQL; on MySQL/MariaDB the filter
// reads a utf8mb4_bin (PAD SPACE) view, so comparisons and endsWith ignore
// trailing spaces there, while the storage's own lookups stay exact. A future
// NO PAD view or translator fix is meant to flip the MySQL expectations.
func checkTrailingSpaceFilters(t *testing.T, store scheduler.Storage, dialect sqldb.Dialect) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "pad", Status: scheduler.TaskStatusActive, Description: "a",
	}}))
	require.NoError(t, store.AddHistory(ctx, &scheduler.TaskHistory{ID: "padh", TaskID: "pad", RunID: "r", StartedAt: 1, EndedAt: 2, Error: "e"}))
	padded := dialect == sqldb.DialectMySQL
	count := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	for expr, want := range map[string]int{
		`id == "pad " && description == "a"`:        count(padded),
		`id == "pad" && description == "a "`:        count(padded),
		`id == "pad" && description.endsWith("a ")`: count(padded),
		`id == "pad" && description.endsWith("A")`:  0,
		`id == "pad" && description == "a"`:         1,
	} {
		page, err := store.TasksPaginated(ctx, scheduler.Pagination{Limit: 10}, parse(t, expr))
		require.NoError(t, err, expr)
		require.Len(t, page, want, expr)
	}
	for expr, want := range map[string]int{
		`error == "e "`:                count(padded),
		`runId.endsWith("r ")`:         count(padded),
		`error == "E"`:                 0,
		`runId == "r" && id == "padh"`: 1,
	} {
		page, err := store.HistoryPaginated(ctx, "pad", scheduler.HistoryPagination{Pagination: scheduler.Pagination{Limit: 10}}, parse(t, expr))
		require.NoError(t, err, expr)
		require.Len(t, page, want, expr)
	}
	got, err := store.GetTask(ctx, "pad ")
	require.NoError(t, err)
	require.Nil(t, got, "storage lookups compare exactly")
}

func parse(t *testing.T, expr string) filter.Node {
	t.Helper()
	p, err := filter.NewParser()
	require.NoError(t, err)
	n, err := p.Parse(t.Context(), expr)
	require.NoError(t, err)
	return n
}

// TestSQLMySQLBinarySchema pins that EnsureSchema declares every string column
// of a fresh MySQL/MariaDB schema as a binary type.
func TestSQLMySQLBinarySchema(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		if b.sql == nil || b.sql.dialect != sqldb.DialectMySQL {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			db, tasks, history := b.sql.open(t)
			store, err := sqldb.New(db, b.sql.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(ctx))
			require.Equal(t, map[string]string{
				"id": "varbinary", "description": "mediumblob", "schedule": "varbinary", "last_run_id": "varbinary",
				"run_lease_id": "varbinary", "meta": "mediumblob",
			}, stringColumnTypes(t, db, tasks))
			require.Equal(t, map[string]string{"id": "varbinary", "task_id": "varbinary", "run_id": "varbinary", "error": "longblob"},
				stringColumnTypes(t, db, history))
		})
	}
}

// stringColumnTypes returns the data type of every non-numeric column of a
// MySQL table in the connection's database.
func stringColumnTypes(t *testing.T, db *sql.DB, table string) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT COLUMN_NAME, DATA_TYPE FROM information_schema.COLUMNS"+
		" WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND DATA_TYPE NOT IN ('int', 'bigint', 'tinyint')", table)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	types := map[string]string{}
	for rows.Next() {
		var name, typ string
		require.NoError(t, rows.Scan(&name, &typ))
		types[strings.ToLower(name)] = strings.ToLower(typ)
	}
	require.NoError(t, rows.Err())
	return types
}

// legacySchema is the MySQL/MariaDB DDL of the previous release: utf8mb4
// columns with the engine's NO PAD binary collation. %[1]s is the tasks table,
// %[2]s the history table, %[3]s the collation.
const legacySchema = `CREATE TABLE %[1]s (
  id              VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL PRIMARY KEY,
  description     MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL,
  status          INT     NOT NULL,
  priority        INT     NOT NULL DEFAULT 0,
  schedule        VARCHAR(1024) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL DEFAULT '',
  last_run_at     BIGINT  NOT NULL DEFAULT 0,
  next_run_at     BIGINT  NOT NULL DEFAULT 0,
  last_run_id     VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL DEFAULT '',
  run_started_at  BIGINT  NOT NULL DEFAULT 0,
  run_lease_until BIGINT  NOT NULL DEFAULT 0,
  run_lease_id    VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL DEFAULT '',
  run_at          BIGINT  NOT NULL DEFAULT 0,
  failures        INT     NOT NULL DEFAULT 0,
  skip_next_run   BOOLEAN NOT NULL DEFAULT FALSE,
  disable_history BOOLEAN NOT NULL DEFAULT FALSE,
  unmanaged       BOOLEAN NOT NULL DEFAULT FALSE,
  one_shot        BOOLEAN NOT NULL DEFAULT FALSE,
  meta            MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL,
  created_at      BIGINT  NOT NULL DEFAULT 0,
  updated_at      BIGINT  NOT NULL DEFAULT 0,
  revision        BIGINT  NOT NULL DEFAULT 0,
  INDEX %[1]s_due_idx (status, next_run_at)
) ENGINE=InnoDB;
CREATE TABLE %[2]s (
  id          VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL PRIMARY KEY,
  task_id     VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL,
  run_id      VARCHAR(255) CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL DEFAULT '',
  error       LONGTEXT CHARACTER SET utf8mb4 COLLATE %[3]s NOT NULL,
  started_at  BIGINT  NOT NULL,
  ended_at    BIGINT  NOT NULL,
  duration_ms BIGINT  NOT NULL DEFAULT 0,
  success     BOOLEAN NOT NULL,
  INDEX %[2]s_task_idx (task_id, started_at DESC, id DESC),
  INDEX %[2]s_ended_idx (ended_at)
) ENGINE=InnoDB`

// legacyStorage builds a storage over tables created with the previous
// release's collated DDL and brought up to date by EnsureSchema (twice).
func (s *sqlTarget) legacyStorage(tb testing.TB) (scheduler.Storage, *sql.DB, string) {
	tb.Helper()
	db, tasks, history := s.open(tb)
	var version string
	require.NoError(tb, db.QueryRowContext(tb.Context(), "SELECT VERSION()").Scan(&version))
	collation := "utf8mb4_0900_bin"
	if strings.Contains(strings.ToLower(version), "mariadb") {
		collation = "utf8mb4_nopad_bin"
	}
	for stmt := range strings.SplitSeq(fmt.Sprintf(legacySchema, tasks, history, collation), ";\n") {
		_, err := db.ExecContext(tb.Context(), stmt)
		require.NoError(tb, err)
	}
	store, err := sqldb.New(db, s.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	return store, db, tasks
}

// TestSQLLegacyCollatedSchema pins that tables created by the previous release
// — utf8mb4 columns with NO PAD binary collations — keep working: EnsureSchema
// leaves their column types alone, and the whole storage contract, the
// SQL-only contracts and the filter semantics hold on them.
func TestSQLLegacyCollatedSchema(t *testing.T) {
	t.Parallel()
	sqlOnly := map[string]func(*testing.T, scheduler.Storage){
		"Identity":   storagetest.Identity,
		"Pagination": storagetest.Pagination,
		"History":    storagetest.History,
	}
	for _, b := range backends() {
		if b.sql == nil || b.sql.dialect != sqldb.DialectMySQL {
			continue
		}
		legacy := func(tb testing.TB) scheduler.Storage {
			store, _, _ := b.sql.legacyStorage(tb)
			return store
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, legacy)
			for name, check := range sqlOnly {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					check(t, legacy(t))
				})
			}
			t.Run("SchemaAndFilters", func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()
				store, db, tasks := b.sql.legacyStorage(t)
				types := stringColumnTypes(t, db, tasks)
				require.Equal(t, "varchar", types["id"], "EnsureSchema must not rewrite legacy column types")
				require.Equal(t, "mediumtext", types["description"])

				require.NoError(t, store.UpsertTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
					ID: "cafe", Status: scheduler.TaskStatusActive, Description: "café",
				}}))
				for expr, want := range map[string]int{
					`description.endsWith("é")`: 1,
					`description.size() == 4`:   1,
					`description.matches("^C")`: 0,
					`description.matches("^c")`: 1,
				} {
					page, err := store.TasksPaginated(ctx, scheduler.Pagination{Limit: 10}, parse(t, expr))
					require.NoError(t, err, expr)
					require.Len(t, page, want, expr)
				}
				checkTrailingSpaceFilters(t, store, b.sql.dialect)
			})
		})
	}
}

// TestSQLConcurrentEnsureSchema pins that instances starting together can each
// create the same absent tables — whether they name them schema-qualified or
// not: EnsureSchema serializes the initial DDL rather than racing on the
// catalog.
func TestSQLConcurrentEnsureSchema(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		if b.sql == nil {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			for range 5 {
				db, tasks, history := b.sql.open(t)
				qualifiedTasks, qualifiedHistory := b.sql.qualify(t, db, tasks), b.sql.qualify(t, db, history)
				var wg sync.WaitGroup
				errs := make([]error, 8)
				for i := range errs {
					wg.Go(func() {
						// Half the instances name the tables schema-qualified.
						tasks, history := tasks, history
						if i%2 == 1 {
							tasks, history = qualifiedTasks, qualifiedHistory
						}
						store, err := sqldb.New(db, b.sql.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
						if err == nil {
							err = store.EnsureSchema(t.Context())
						}
						errs[i] = err
					})
				}
				wg.Wait()
				for _, err := range errs {
					require.NoError(t, err)
				}
			}
		})
	}
}

// qualify returns table qualified by the connection's current schema
// (PostgreSQL) or database (MySQL/MariaDB).
func (s *sqlTarget) qualify(tb testing.TB, db *sql.DB, table string) string {
	tb.Helper()
	query := "SELECT DATABASE()"
	if s.dialect == sqldb.DialectPostgres {
		query = "SELECT current_schema()"
	}
	var schema string
	require.NoError(tb, db.QueryRowContext(tb.Context(), query).Scan(&schema))
	return schema + "." + table
}

// TestSQLEnsureSchemaDuringDeleteTask pins that a PostgreSQL EnsureSchema on
// existing tables cannot deadlock with DeleteTask. EnsureSchema runs its DDL in
// one transaction, so it must take the table locks in DeleteTask's order —
// history, then tasks. The test holds a delete transaction on history, lets
// EnsureSchema block behind it, then deletes from tasks: with tasks locked
// first the server would abort one side as a deadlock.
func TestSQLEnsureSchemaDuringDeleteTask(t *testing.T) {
	t.Parallel()
	for _, b := range backends() {
		if b.sql == nil || b.sql.dialect != sqldb.DialectPostgres {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			db, tasks, history := b.sql.open(t)
			store, err := sqldb.New(db, b.sql.dialect, sqldb.WithTasksTable(tasks), sqldb.WithHistoryTable(history))
			require.NoError(t, err)
			require.NoError(t, store.EnsureSchema(ctx))

			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, "DELETE FROM "+history+" WHERE task_id = $1", "x")
			require.NoError(t, err)

			done := make(chan error, 1)
			go func() { done <- store.EnsureSchema(ctx) }()
			require.Eventually(t, func() bool {
				var waiting int
				err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE NOT granted AND relation = $1::regclass", history).Scan(&waiting)
				return err == nil && waiting > 0
			}, 10*time.Second, 10*time.Millisecond, "EnsureSchema must wait on the history table")

			_, err = tx.ExecContext(ctx, "DELETE FROM "+tasks+" WHERE id = $1", "x")
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
			require.NoError(t, <-done)
		})
	}
}
