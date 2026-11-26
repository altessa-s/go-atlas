// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/store/postgres"
	"github.com/altessa-s/go-atlas/store/postgres/postgrestest"
)

// Postgrestest uses a direct maintenance session for database ownership. PgBouncer
// installations may expose one configured database, so pooled runs use a unique table there.
type uowFixture struct {
	db    *sql.DB
	q     postgres.Executor
	table string
	c     *postgres.Committer
	r     *uow.Runner
}

func newUOWFixture(t testing.TB, pooled bool, opts ...postgres.CommitterOption) *uowFixture {
	t.Helper()
	var db *sql.DB
	if pooled {
		dsn := os.Getenv("PGBOUNCER_DSN")
		if dsn == "" {
			t.Skip("PGBOUNCER_DSN not configured")
		}
		var err error
		db, err = postgres.Open(t.Context(), redacted.RedactedString(dsn), postgres.WithPooling(postgres.PoolingTransaction))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
	} else {
		db = postgrestest.New(t)
	}
	table := fixturePrefix() + "items"
	_, err := db.ExecContext(t.Context(), "CREATE TABLE "+table+" (id text PRIMARY KEY DEFERRABLE INITIALLY DEFERRED, v int NOT NULL DEFAULT 0)")
	require.NoError(t, err)
	if pooled {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
			defer cancel()
			_, err := db.ExecContext(ctx, "DROP TABLE "+table)
			require.NoError(t, err)
		})
	}
	c, err := postgres.NewCommitter(db, opts...)
	require.NoError(t, err)
	return &uowFixture{db: db, q: postgres.Querier(db), table: table, c: c, r: uow.New(c, nil, uow.WithDetector(postgres.Transaction))}
}

func (f *uowFixture) insert(ctx context.Context, id string) error {
	_, err := f.q.ExecContext(ctx, "INSERT INTO "+f.table+" (id) VALUES ($1)", id)
	return err
}

func TestUOW(t *testing.T) {
	t.Parallel()
	for _, pooled := range []bool{false, true} {
		mode := "direct"
		if pooled {
			mode = "pgbouncer"
		}
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				name string
				run  func(testing.TB, *uowFixture)
			}{
				{"nested commit", nestedCommit}, {"nested abort", nestedAbort}, {"foreign", foreign}, {"another pool", anotherPool}, {"serialization", serialization},
				{"lost backend", lostBackend}, {"panic", panicRollback}, {"retained", retained}, {"deferred constraint", deferredConstraint}, {"savepoint", savepoint},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					var opts []postgres.CommitterOption
					if tc.name == "serialization" {
						opts = append(opts, postgres.WithTxRetries(1))
					}
					tc.run(t, newUOWFixture(t, pooled, opts...))
				})
			}
		})
	}
}

func nestedCommit(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r

	hookSaw := -1
	require.NoError(t, r.Run(t.Context(), func(ctx context.Context) error {
		return r.Run(ctx, func(nctx context.Context) error {
			if err := pg.insert(nctx, "nested"); err != nil {
				return err
			}
			return r.AfterCommit(nctx, func(hctx context.Context) {
				var n int
				if err := pg.q.QueryRowContext(hctx, `SELECT count(*) FROM `+pg.table+` WHERE id = 'nested'`).Scan(&n); err == nil {
					hookSaw = n
				}
			})
		})
	}))
	require.Equal(t, 1, hookSaw, "the hook observes the committed write")
}

func nestedAbort(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r

	errAbort := errors.New("abort")
	hooks := 0
	err := r.Run(t.Context(), func(ctx context.Context) error {
		if err := r.Run(ctx, func(nctx context.Context) error {
			if err := pg.insert(nctx, "rolled-back"); err != nil {
				return err
			}
			return r.AfterCommit(nctx, func(context.Context) { hooks++ })
		}); err != nil {
			return err
		}
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	require.Zero(t, hooks)
	require.Zero(t, count(t, t.Context(), pg.db, pg.table))
}

func foreign(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r

	errRollback := errors.New("rollback")
	var runErr, hookErr error
	err := pg.c.WithTransaction(t.Context(), func(ctx context.Context) error {
		runErr = r.Run(ctx, func(inner context.Context) error { return pg.insert(inner, "foreign") })
		hookErr = r.AfterCommit(ctx, func(context.Context) {})
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	require.ErrorIs(t, runErr, uow.ErrForeignTransaction)
	require.ErrorIs(t, hookErr, uow.ErrForeignTransaction)
	require.Zero(t, count(t, t.Context(), pg.db, pg.table))
}

func anotherPool(t testing.TB, pg *uowFixture) {
	t.Helper()
	other, err := sql.Open("pgx", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	require.NoError(t, pg.r.Run(t.Context(), func(ctx context.Context) error {
		err := postgres.WithTransaction(ctx, other, func(context.Context) error { t.Fatal("foreign body ran"); return nil })
		require.ErrorIs(t, err, postgres.ErrDatabaseMismatch)
		return pg.insert(ctx, "outer")
	}))
	require.Equal(t, 1, count(t, t.Context(), pg.db, pg.table))
}

// A real serialization failure (SQLSTATE 40001) reruns the body in a new transaction; uow applies only the registrations of the
// committed attempt.
func serialization(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r
	require.NoError(t, pg.insert(t.Context(), "counter"))

	var (
		attempts atomic.Int32
		applied  []int32
	)
	require.NoError(t, r.Run(t.Context(), func(ctx context.Context) error {
		n := attempts.Add(1)
		if err := r.AfterCommit(ctx, func(context.Context) { applied = append(applied, n) }); err != nil {
			return err
		}
		if _, err := pg.q.ExecContext(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); err != nil {
			return err
		}
		var v int
		if err := pg.q.QueryRowContext(ctx, `SELECT v FROM `+pg.table+` WHERE id = 'counter'`).Scan(&v); err != nil {
			return err
		}
		if n == 1 {
			// A concurrent writer commits after this transaction's snapshot was taken.
			if _, err := pg.db.ExecContext(ctx, `UPDATE `+pg.table+` SET v = v + 1 WHERE id = 'counter'`); err != nil {
				return err
			}
		}
		_, err := pg.q.ExecContext(ctx, `UPDATE `+pg.table+` SET v = v + 10 WHERE id = 'counter'`)
		if n == 1 {
			kind, _ := postgres.ClassifyContext(ctx, err)
			require.True(t, kind.RetryableTransaction(), "the first attempt must hit a serialization failure, got %v", err)
		}
		return err
	}))

	require.Equal(t, int32(2), attempts.Load())
	require.Equal(t, []int32{2}, applied, "the hook of the discarded attempt never runs")
	var v int
	require.NoError(t, pg.db.QueryRowContext(t.Context(), `SELECT v FROM `+pg.table+` WHERE id = 'counter'`).Scan(&v))
	require.Equal(t, 11, v)
}

// A connection lost during COMMIT leaves the outcome unknown: uow reports UnknownOutcomeError and applies nothing.
func lostBackend(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r

	hooks := 0
	err := r.Run(t.Context(), func(ctx context.Context) error {
		if err := pg.insert(ctx, "lost"); err != nil {
			return err
		}
		var pid int
		if err := pg.q.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		if err := r.AfterCommit(ctx, func(context.Context) { hooks++ }); err != nil {
			return err
		}
		// Kill this transaction's backend from another session and wait until it is gone, so COMMIT meets a dead connection.
		if _, err := pg.db.ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
			return err
		}
		require.Eventually(t, func() bool {
			var alive bool
			err := pg.db.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1)`, pid).Scan(&alive)
			return err == nil && !alive
		}, 10*time.Second, 20*time.Millisecond)
		return nil
	})
	require.ErrorIs(t, err, uow.ErrUnknownOutcome)
	_, ok := errors.AsType[*uow.UnknownOutcomeError](err)
	require.True(t, ok)
	require.Zero(t, hooks)
	require.Zero(t, count(t, t.Context(), pg.db, pg.table), "the server rolled the transaction back with its backend")
}

func panicRollback(t testing.TB, pg *uowFixture) {
	t.Helper()
	r := pg.r

	hooks := 0
	require.PanicsWithValue(t, "boom", func() {
		_ = r.Run(t.Context(), func(ctx context.Context) error {
			if err := pg.insert(ctx, "partial"); err != nil {
				return err
			}
			if err := r.AfterCommit(ctx, func(context.Context) { hooks++ }); err != nil {
				return err
			}
			panic("boom")
		})
	})
	require.Zero(t, hooks)
	require.Zero(t, count(t, t.Context(), pg.db, pg.table))
}

func retained(t testing.TB, pg *uowFixture) {
	t.Helper()

	var bodyCtx context.Context //nolint:fatcontext // retained on purpose to observe the finished transaction
	require.NoError(t, pg.c.WithTransaction(t.Context(), func(ctx context.Context) error {
		bodyCtx = ctx
		require.True(t, postgres.InWriteTransaction(ctx))
		return pg.insert(ctx, "committed")
	}))
	_, ok := pg.c.Detect(bodyCtx)
	require.False(t, ok)
	require.Equal(t, 1, count(t, t.Context(), pg.db, pg.table))
}

func deferredConstraint(t testing.TB, f *uowFixture) {
	t.Helper()
	err := f.r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, f.insert(ctx, "duplicate"))
		require.NoError(t, f.insert(ctx, "duplicate"))
		return f.r.AfterCommit(ctx, func(context.Context) { t.Error("rolled-back hook ran") })
	})
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	kind, _ := postgres.ClassifyContext(t.Context(), err)
	require.Equal(t, postgres.Unique, kind)
	require.Zero(t, count(t, t.Context(), f.db, f.table))
}

func savepoint(t testing.TB, f *uowFixture) {
	t.Helper()
	var hooks, effects []string
	var retained context.Context
	var callbackErr error
	register := func(ctx context.Context, label string) {
		require.NoError(t, f.r.AfterCommit(ctx, func(context.Context) { hooks = append(hooks, label) }))
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{Apply: func(context.Context) error {
			effects = append(effects, label)
			return nil
		}}))
	}
	require.NoError(t, f.r.Run(t.Context(), func(ctx context.Context) error {
		err := postgres.WithSavepoint(ctx, f.db, func(child context.Context) error {
			retained = child
			require.NoError(t, f.insert(child, "discarded"))
			register(child, "discarded")
			err := postgres.WithTransaction(child, f.db, func(joined context.Context) error { _, err := f.q.ExecContext(joined, "SELECT 1/0"); return err })
			require.Equal(t, "22012", sqldialect.SQLState(err))
			_, err = f.q.ExecContext(child, "SELECT 1")
			return err
		})
		require.Equal(t, "25P02", sqldialect.SQLState(err))
		require.ErrorIs(t, f.insert(retained, "late"), sql.ErrTxDone)
		return postgres.WithSavepoint(ctx, f.db, func(child context.Context) error {
			register(child, "released")
			postgres.AfterCommitContext(child, func(fresh context.Context) {
				callbackErr = f.r.Run(fresh, func(independent context.Context) error {
					return f.insert(independent, "from callback")
				})
			})
			require.Empty(t, hooks)
			require.Empty(t, effects)
			return f.insert(child, "recovered")
		})
	}))
	require.Equal(t, []string{"released"}, hooks)
	require.Equal(t, []string{"released"}, effects)
	require.NoError(t, callbackErr)
	for _, expected := range []string{"from callback", "recovered"} {
		var id string
		require.NoError(t, f.db.QueryRowContext(t.Context(), "SELECT id FROM "+f.table+" WHERE id = $1", expected).Scan(&id))
		require.Equal(t, expected, id)
	}
	require.Equal(t, 2, count(t, t.Context(), f.db, f.table))
}
