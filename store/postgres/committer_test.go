// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/store/postgres"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	queuesql "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
)

// Script only the transaction protocol. An unexpected statement can't pass this driver.
type protocol struct {
	driver.Connector
	begin, commit, rollback func() error
}
type protocolConn struct {
	driver.Conn
	p *protocol
}
type protocolTx struct{ p *protocol }

func (p *protocol) Connect(context.Context) (driver.Conn, error) { return protocolConn{p: p}, nil }
func (c protocolConn) Close() error                              { return nil }
func (c protocolConn) Begin() (driver.Tx, error) {
	if c.p.begin != nil {
		if err := c.p.begin(); err != nil {
			return nil, err
		}
	}
	return protocolTx{c.p}, nil
}
func (tx protocolTx) Commit() error {
	if tx.p.commit != nil {
		return tx.p.commit()
	}
	return nil
}
func (tx protocolTx) Rollback() error {
	if tx.p.rollback != nil {
		return tx.p.rollback()
	}
	return nil
}

func newRunner(tb testing.TB, p *protocol, opts ...postgres.CommitterOption) *uow.Runner {
	tb.Helper()
	db := sql.OpenDB(p)
	tb.Cleanup(func() { require.NoError(tb, db.Close()) })
	c, err := postgres.NewCommitter(db, opts...)
	require.NoError(tb, err)
	return uow.New(c, nil, uow.WithDetector(postgres.Transaction))
}

func TestCommitOutcome(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		cause   error
		unknown bool
	}{
		{"serialization", &pgconn.PgError{Code: "40001", Message: "private"}, false}, {"deferred unique", &pgconn.PgError{Code: "23505", Message: "private"}, false},
		{"aborted", pgx.ErrTxCommitRollback, false}, {"connection", &pgconn.PgError{Code: "08006", Message: "private"}, true}, {"disconnect", io.ErrUnexpectedEOF, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRunner(t, &protocol{commit: func() error { return tc.cause }})
			attempts := 0
			err := r.Run(t.Context(), func(ctx context.Context) error {
				attempts++
				return r.AfterCommit(ctx, func(context.Context) { t.Error("hook applied") })
			})
			require.Equal(t, 1, attempts)
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, tc.unknown, errors.Is(err, uow.ErrUnknownOutcome))
			require.NotContains(t, fmt.Sprint(err), "private")
		})
	}
}

func TestRetryCommit(t *testing.T) {
	t.Parallel()
	for _, last := range []string{"success", "begin failure", "canceled", "deadline"} {
		t.Run(last, func(t *testing.T) {
			t.Parallel()
			first := &pgconn.PgError{Code: "40001", Message: "private"}
			beginErr := errors.New("private begin failure")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			p := new(protocol)
			attempts := 0
			p.begin = func() error {
				if attempts == 1 && last == "begin failure" {
					return beginErr
				}
				return nil
			}
			p.commit = func() error {
				if attempts == 1 {
					if last == "canceled" {
						cancel()
					}
					return first
				}
				return nil
			}
			opts := []postgres.CommitterOption{postgres.WithTxRetries(1), postgres.WithTxRetryDelay(time.Microsecond)}
			if last == "deadline" {
				opts = []postgres.CommitterOption{postgres.WithTxRetries(1), postgres.WithTxTimeout(time.Second), postgres.WithTxRetryDelay(time.Hour)}
			}
			r := newRunner(t, p, opts...)
			var hooks []int
			err := r.Run(ctx, func(txCtx context.Context) error {
				attempts++
				n := attempts
				return r.Run(txCtx, func(inner context.Context) error {
					return r.AfterCommit(inner, func(context.Context) { hooks = append(hooks, n) })
				})
			})
			if last == "success" {
				require.NoError(t, err)
				require.Equal(t, []int{2}, hooks)
				return
			}
			require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
			require.Empty(t, hooks)
			require.Equal(t, 1, attempts)
			require.NotContains(t, err.Error(), "private")
			if last == "begin failure" {
				require.ErrorIs(t, err, beginErr)
				return
			}
			if last == "deadline" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.ErrorIs(t, err, first)
			kind, _ := postgres.Classify(err)
			require.False(t, kind.RetryableTransaction())
		})
	}
}

func TestRetainedTransaction(t *testing.T) {
	t.Parallel()
	for _, end := range []string{"commit", "rollback", "cancel"} {
		t.Run(end, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			c, err := postgres.NewCommitter(db)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var retained context.Context //nolint:fatcontext // exercising a retained transaction scope
			abort := errors.New("abort")
			check := func() {
				_, ok := c.Detect(retained)
				require.False(t, ok)
				require.False(t, postgres.InWriteTransaction(retained))
				tx, open := postgres.Tx(retained)
				require.False(t, open)
				require.NotNil(t, tx)
				for _, q := range []postgres.DBTX{postgres.Queryer(retained, db), postgres.Querier(db)} {
					_, execErr := q.ExecContext(retained, "late")
					require.ErrorIs(t, execErr, sql.ErrTxDone)
				}
				_, groupResolver := (postgres.Pools{postgres.DefaultPool: db}).Binding(postgres.DefaultPool)
				for _, resolve := range []func(context.Context) (*sql.Tx, bool){postgres.Tx, groupResolver} {
					pool := sqldialect.Postgres.WrapPool(db).WithResolver(resolve)
					_, execErr := pool.ExecContext(retained, "late")
					require.ErrorIs(t, execErr, sql.ErrTxDone)
					_, queryErr := pool.QueryContext(retained, "late")
					require.ErrorIs(t, queryErr, sql.ErrTxDone)
					require.ErrorIs(t, pool.QueryRowContext(retained, "late").Scan(), sql.ErrTxDone)
					_, beginErr := pool.BeginScope(retained, nil)
					require.ErrorIs(t, beginErr, sql.ErrTxDone)
					events, err := outboxsql.New(db, outboxsql.DialectPostgres, outboxsql.WithTxResolver(resolve))
					require.NoError(t, err)
					require.ErrorIs(t, events.SaveEvents(retained, outbox.Event{Id: "late", Key: "late"}), sql.ErrTxDone)
					// A multi-chunk requeue normally starts its own transaction.
					queue, err := queuesql.New[string](db, queuesql.DialectPostgres, queuesql.WithTxResolver(resolve))
					require.NoError(t, err)
					_, err = queue.Requeue(retained, time.Now(), make([]string, 1001)...)
					require.ErrorIs(t, err, sql.ErrTxDone)
				}
				require.ErrorIs(t, c.WithTransaction(retained, func(context.Context) error { t.Fatal("reopened transaction"); return nil }), sql.ErrTxDone)
			}
			err = c.WithTransaction(ctx, func(inner context.Context) error {
				retained = context.WithoutCancel(inner)
				if end == "cancel" {
					cancel()
					check()
				}
				if end == "rollback" {
					return abort
				}
				return nil
			})
			switch end {
			case "commit":
				require.NoError(t, err)
			case "rollback":
				require.ErrorIs(t, err, abort)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}
			check()
			require.Empty(t, fake.Calls())
		})
	}
}

func TestReplayRestrictions(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	other, durable := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db, postgres.WithTxRetries(1))
	require.NoError(t, err)
	calls := 0
	require.NoError(t, c.WithTransaction(t.Context(), func(ctx context.Context) error {
		calls++
		require.ErrorIs(t, postgres.WithTransaction(ctx, other, func(context.Context) error { t.Fatal("independent body ran"); return nil }), postgres.ErrDifferentDatabase)
		_, err := postgres.BeginAsyncCommit(ctx, other)
		require.ErrorIs(t, err, postgres.ErrUnsafeReplay)
		require.ErrorIs(t, c.WithTransaction(ctx, func(context.Context) error { t.Fatal("replayed a join"); return nil }), postgres.ErrUnsafeReplay)
		if calls == 1 {
			return &pgconn.PgError{Code: "40P01"}
		}
		return nil
	}))
	require.Equal(t, 2, calls)
	require.Zero(t, durable.Commits())
}

func TestJoinedPanic(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)
	r := uow.New(c, nil, uow.WithDetector(postgres.Transaction))
	err = r.Run(t.Context(), func(ctx context.Context) error {
		require.PanicsWithValue(t, "private panic", func() {
			_ = postgres.WithSavepoint(ctx, db, func(child context.Context) error {
				return postgres.WithTransaction(child, db, func(context.Context) error { panic("private panic") })
			})
		})
		return r.AfterCommit(ctx, func(context.Context) { t.Error("hook applied") })
	})
	require.ErrorIs(t, err, postgres.ErrTransactionPanicked)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.NotContains(t, err.Error(), "private")
	require.Equal(t, 1, fake.Rollbacks())
	require.Zero(t, fake.Commits())
}

func TestTransactionDiagnostics(t *testing.T) {
	t.Parallel()
	bodyErr := &pgconn.PgError{Code: "23505", Message: "private body"}
	rollbackErr := errors.New("private rollback")
	r := newRunner(t, &protocol{rollback: func() error { return rollbackErr }})
	err := r.Run(t.Context(), func(context.Context) error { return bodyErr })
	require.ErrorIs(t, err, rollbackErr)
	got, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok)
	require.Same(t, bodyErr, got)
	require.NotContains(t, fmt.Sprintf("%v %+v %#v", err, err, err), "private")
}
func TestWithTransaction_RetryClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		retries      int
		failures     int
		err          error
		wantAttempts int
		wantErr      bool
	}{
		{"serialization failure is rerun", 3, 1, &pgconn.PgError{Code: "40001"}, 2, false},
		{"deadlock is rerun", 3, 2, &pgconn.PgError{Code: "40P01"}, 3, false},
		{"wrapped serialization failure is rerun", 3, 1, fmt.Errorf("insert: %w", &pgconn.PgError{Code: "40001"}), 2, false},
		{"reruns are bounded", 2, 10, &pgconn.PgError{Code: "40001"}, 3, true},
		{"zero retries disables reruns", 0, 1, &pgconn.PgError{Code: "40001"}, 1, true},
		{"unique violation is not rerun", 3, 1, &pgconn.PgError{Code: "23505"}, 1, true},
		{"plain error is not rerun", 3, 1, errors.New("plain"), 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			p, err := postgres.NewCommitter(db, postgres.WithTxRetries(tc.retries))
			require.NoError(t, err)

			var (
				attempts int
				ids      []any
			)
			err = p.WithTransaction(t.Context(), func(ctx context.Context) error {
				attempts++
				id, ok := postgres.Transaction(ctx)
				require.True(t, ok)
				ids = append(ids, id)
				if attempts <= tc.failures {
					return tc.err
				}
				return nil
			})
			require.Equal(t, tc.wantAttempts, attempts)
			failed := tc.wantAttempts
			if tc.wantErr {
				require.ErrorIs(t, err, tc.err)
				require.Zero(t, fake.Commits())
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, fake.Commits())
				failed--
			}
			require.Equal(t, failed, fake.Rollbacks(), "every failed attempt is rolled back")
			for i := 1; i < len(ids); i++ {
				require.NotSame(t, ids[i-1], ids[i], "every rerun is a new transaction")
			}
		})
	}
}

func TestWithTransaction_TimeoutCapsTheTransaction(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	p, err := postgres.NewCommitter(db, postgres.WithTxTimeout(time.Hour))
	require.NoError(t, err)

	require.NoError(t, p.WithTransaction(t.Context(), func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(time.Hour), deadline, time.Minute)
		return nil
	}))

	short, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	want, _ := short.Deadline()
	require.NoError(t, p.WithTransaction(short, func(ctx context.Context) error {
		deadline, _ := ctx.Deadline()
		require.Equal(t, want, deadline, "an earlier caller deadline wins")
		return nil
	}))
}

func TestCommitterOptions(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	for _, tc := range []struct {
		name string
		db   *sql.DB
		opts []postgres.CommitterOption
	}{
		{"nil pool", nil, nil}, {"negative retries", db, []postgres.CommitterOption{postgres.WithTxRetries(-1)}},
		{"negative delay", db, []postgres.CommitterOption{postgres.WithTxRetryDelay(-1)}}, {"negative timeout", db, []postgres.CommitterOption{postgres.WithTxTimeout(-1)}},
		{"zero delay with replay", db, []postgres.CommitterOption{postgres.WithTxRetries(1), postgres.WithTxRetryDelay(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := postgres.NewCommitter(tc.db, tc.opts...)
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
		})
	}
}

func TestDetectSnapshotAndPoolRoute(t *testing.T) {
	t.Parallel()
	base, _ := testhelpers.NewFakeSQL(t, nil)
	worker, _ := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(base)
	require.NoError(t, err)
	routed := postgres.WithPool(t.Context(), base, worker)
	require.NoError(t, c.WithTransaction(routed, func(ctx context.Context) error {
		owner, ok := c.Detect(ctx)
		require.True(t, ok)
		return postgres.WithSavepoint(ctx, base, func(child context.Context) error {
			id, ok := postgres.Transaction(child)
			require.True(t, ok)
			require.Same(t, owner, id)
			return nil
		})
	}))
	require.NoError(t, postgres.WithSnapshot(routed, base, func(ctx context.Context) error {
		_, open := postgres.Tx(ctx)
		require.True(t, open)
		_, write := postgres.Transaction(ctx)
		require.False(t, write)
		_, write = c.Detect(ctx)
		require.False(t, write)
		require.Error(t, c.WithTransaction(ctx, func(context.Context) error { t.Fatal("write unit joined snapshot"); return nil }))
		return nil
	}))
}

func TestRecoveredNestedRunnerPanic(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)
	r := uow.New(c, nil, uow.WithDetector(postgres.Transaction))
	err = r.Run(t.Context(), func(ctx context.Context) error {
		require.NoError(t, r.AfterCommit(ctx, func(context.Context) { t.Error("hook ran") }))
		require.PanicsWithValue(t, "private", func() { _ = r.Run(ctx, func(context.Context) error { panic("private") }) })
		return nil
	})
	require.ErrorIs(t, err, uow.ErrScopeClosed)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.Equal(t, 1, fake.Rollbacks())
	require.Zero(t, fake.Commits())
}

func TestCancelDuringReplayBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(t.Context())
		secret := errors.New("private cancellation cause")
		defer cancel(nil)
		failure := &pgconn.PgError{Code: "40001", Message: "private commit"}
		r := newRunner(t, &protocol{commit: func() error { return failure }}, postgres.WithTxRetries(1), postgres.WithTxRetryDelay(time.Hour))
		result := make(chan error, 1)
		go func() { result <- r.Run(ctx, func(context.Context) error { return nil }) }()
		synctest.Wait()
		cancel(secret)
		err := <-result
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, failure)
		require.ErrorIs(t, err, secret)
		require.NotContains(t, err.Error(), "private")
		require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
		kind, _ := postgres.Classify(err)
		require.False(t, kind.RetryableTransaction())
	})
}

func TestSavepointUnitOfWorkRegistrations(t *testing.T) {
	t.Parallel()

	abort := errors.New("abort operation")
	for _, tc := range []struct {
		name              string
		childErr, rootErr error
		want              []string
	}{
		{"released", nil, nil, []string{"child hook", "root hook", "grandchild effect", "child effect"}},
		{"rolled back", abort, nil, []string{"root hook"}},
		{"root aborted", nil, abort, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := testhelpers.NewFakeSQL(t, nil)
			c, err := postgres.NewCommitter(db)
			require.NoError(t, err)
			r := uow.New(c, nil, uow.WithDetector(postgres.Transaction))
			var applied []string
			effect := func(label string) uow.Effect {
				return uow.Effect{Label: label, Apply: func(context.Context) error {
					applied = append(applied, label)
					return nil
				}}
			}
			err = r.Run(t.Context(), func(root context.Context) error {
				var retained context.Context
				saveErr := postgres.WithSavepoint(root, db, func(child context.Context) error {
					retained = child
					require.NoError(t, r.AfterCommit(child, func(context.Context) { applied = append(applied, "child hook") }))
					require.NoError(t, r.AfterCommit(root, func(context.Context) { applied = append(applied, "root hook") }))
					require.NoError(t, postgres.WithSavepoint(child, db, func(grandchild context.Context) error {
						return uow.OnCommit(grandchild, effect("grandchild effect"))
					}))
					require.NoError(t, uow.OnCommit(child, effect("child effect")))
					return tc.childErr
				})
				require.ErrorIs(t, saveErr, tc.childErr)
				require.ErrorIs(t, r.AfterCommit(retained, func(context.Context) {}), uow.ErrScopeClosed)
				require.ErrorIs(t, uow.OnCommit(retained, effect("late effect")), uow.ErrScopeClosed)
				require.Empty(t, applied)
				return tc.rootErr
			})
			require.ErrorIs(t, err, tc.rootErr)
			require.Equal(t, tc.want, applied)
		})
	}
}

func TestFailedReleaseDiscardsUnitOfWorkEffect(t *testing.T) {
	t.Parallel()
	failure := errors.New("release failed")
	releases := 0
	db, _ := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		if query == "RELEASE SAVEPOINT atlas_nested_operation" {
			releases++
			if releases == 1 {
				return testhelpers.FakeSQLReply{Err: failure}
			}
		}
		return testhelpers.FakeSQLReply{}
	})
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)
	r := uow.New(c, nil)
	err = r.Run(t.Context(), func(root context.Context) error {
		saveErr := postgres.WithSavepoint(root, db, func(child context.Context) error {
			return uow.OnCommit(child, uow.Effect{Apply: func(context.Context) error {
				t.Error("effect ran after unsuccessful savepoint release")
				return nil
			}})
		})
		require.ErrorIs(t, saveErr, failure)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, releases)
}

func TestFinishedSavepointCannotExecuteOrRegisterHooks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "released"},
		{name: "rolled back", err: errors.New("abort savepoint")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			wake := make(chan struct{}, 1)
			require.NoError(t, postgres.WithTransaction(t.Context(), db, func(root context.Context) error {
				var retained context.Context
				var bound postgres.DBTX
				require.ErrorIs(t, postgres.WithSavepoint(root, db, func(child context.Context) error {
					retained = child
					bound = postgres.Queryer(child, db)
					_, err := bound.ExecContext(child, "child operation")
					require.NoError(t, err)
					return tc.err
				}), tc.err)
				owner, open := postgres.Tx(root)
				require.True(t, open)
				closed, open := postgres.Tx(retained)
				require.Same(t, owner, closed)
				require.False(t, open)
				require.False(t, postgres.InWriteTransaction(retained))
				calls := fake.Calls()
				for _, q := range []postgres.DBTX{bound, postgres.Querier(db)} {
					_, err := q.ExecContext(retained, "late")
					require.ErrorIs(t, err, sql.ErrTxDone)
					_, err = q.PrepareContext(retained, "late")
					require.ErrorIs(t, err, sql.ErrTxDone)
					_, err = q.QueryContext(retained, "late")
					require.ErrorIs(t, err, sql.ErrTxDone)
					require.ErrorIs(t, q.QueryRowContext(retained, "late").Scan(), sql.ErrTxDone)
				}
				_, err := bound.ExecContext(root, "late using parent context")
				require.ErrorIs(t, err, sql.ErrTxDone)
				require.Equal(t, calls, fake.Calls())
				postgres.AfterCommit(retained, func() { t.Error("closed savepoint registered a hook") })
				postgres.NotifyAfterCommit(retained, wake)
				_, err = postgres.Querier(db).ExecContext(root, "parent operation")
				return err
			}))
			require.Equal(t, 1, fake.Commits())
			require.Zero(t, fake.Rollbacks())
			require.Empty(t, wake)
		})
	}
}

func TestAfterCommitContextStartsFreshUnitOfWork(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)
	r := uow.New(c, nil)
	type requestKey struct{}
	ctx := context.WithValue(t.Context(), requestKey{}, "request")
	var callbackErr error
	called := false
	require.NoError(t, r.Run(ctx, func(root context.Context) error {
		return postgres.WithSavepoint(root, db, func(child context.Context) error {
			postgres.AfterCommitContext(child, func(fresh context.Context) {
				called = true
				require.Equal(t, "request", fresh.Value(requestKey{}))
				_, deadline := fresh.Deadline()
				require.False(t, deadline)
				require.NoError(t, fresh.Err())
				require.ErrorIs(t, uow.OnCommit(fresh, uow.Effect{}), uow.ErrNoUnitOfWork)
				require.Equal(t, 1, fake.Commits())
				callbackErr = r.Run(fresh, func(inner context.Context) error {
					return uow.OnCommit(inner, uow.Effect{Apply: func(context.Context) error {
						require.Equal(t, 2, fake.Commits())
						return nil
					}})
				})
			})
			return nil
		})
	}))
	require.True(t, called)
	require.NoError(t, callbackErr)
	require.Equal(t, 2, fake.Commits())
}

func TestUnitOfWorkRejectsOpenTransactionByDefault(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"root transaction", "savepoint", "snapshot"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			c, err := postgres.NewCommitter(db)
			require.NoError(t, err)
			r := uow.New(c, nil, uow.WithRunHooksOutsideTransaction())
			check := func(ctx context.Context) error {
				require.ErrorIs(t, r.Run(ctx, func(context.Context) error {
					t.Error("unit body joined someone else's transaction")
					return nil
				}), uow.ErrForeignTransaction)
				require.ErrorIs(t, r.AfterCommit(ctx, func(context.Context) {
					t.Error("hook ran before its transaction committed")
				}), uow.ErrForeignTransaction)
				require.Zero(t, fake.Commits())
				return nil
			}
			switch name {
			case "root transaction":
				require.NoError(t, postgres.WithTransaction(t.Context(), db, check))
			case "savepoint":
				require.NoError(t, postgres.WithTransaction(t.Context(), db, func(ctx context.Context) error {
					return postgres.WithSavepoint(ctx, db, check)
				}))
			case "snapshot":
				require.NoError(t, postgres.WithSnapshot(t.Context(), db, check))
			}
			require.Equal(t, 1, fake.Commits())
		})
	}
}

func TestUnitOfWorkAppliesEffectsAfterObserverPanic(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db, postgres.WithTxRetries(1), postgres.WithTxRetryDelay(time.Microsecond))
	require.NoError(t, err)
	r := uow.New(c, nil)
	effectErr := errors.New("effect failed")
	wake := make(chan struct{}, 1)
	var calls []string
	attempts := 0

	err = r.Run(t.Context(), func(ctx context.Context) error {
		attempts++
		postgres.AfterCommit(ctx, func() {
			calls = append(calls, "native observer")
			// SQLSTATE 40001 is only replayable before a successful COMMIT.
			panic(&pgconn.PgError{Code: "40001", Message: "private observer data"})
		})
		postgres.AfterCommit(ctx, func() { calls = append(calls, "next native observer") })
		postgres.NotifyAfterCommit(ctx, wake)
		require.NoError(t, uow.OnCommit(ctx, uow.Effect{Apply: func(context.Context) error {
			require.Equal(t, 1, fake.Commits())
			require.Len(t, wake, 1)
			calls = append(calls, "effect")
			return effectErr
		}}))
		return r.AfterCommit(ctx, func(context.Context) { calls = append(calls, "unit hook") })
	})

	require.ErrorIs(t, err, postgres.ErrPostCommitObserverPanicked)
	require.ErrorIs(t, err, effectErr)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.NotContains(t, err.Error(), "private observer data")
	require.Equal(t, 1, attempts)
	require.Equal(t, []string{"native observer", "next native observer", "effect", "unit hook"}, calls)
}

type committerWrapper struct {
	*postgres.Committer
	withTransaction func(context.Context, func(context.Context) error) error
}

func (w committerWrapper) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return w.withTransaction(ctx, fn)
}

func TestUnitOfWorkHonorsEmbeddedCommitterOverride(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)

	rejected := errors.New("transaction rejected by wrapper")
	r := uow.New(committerWrapper{c, func(context.Context, func(context.Context) error) error {
		return rejected
	}}, nil)
	var bodyCalled bool
	require.ErrorIs(t, r.Run(t.Context(), func(context.Context) error {
		bodyCalled = true
		return nil
	}), rejected)
	require.False(t, bodyCalled)
	require.Zero(t, fake.Commits())
}

func TestUnitOfWorkHonorsWrappedPostCommitOutcome(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"committer", "package transaction"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			c, err := postgres.NewCommitter(db)
			require.NoError(t, err)
			var wrapperCalled, effectCalled bool
			r := uow.New(committerWrapper{c, func(ctx context.Context, fn func(context.Context) error) error {
				wrapperCalled = true
				run := c.WithTransaction
				if route == "package transaction" {
					run = func(ctx context.Context, fn func(context.Context) error) error {
						return postgres.WithTransaction(ctx, db, fn)
					}
				}
				if err := run(ctx, fn); err != nil {
					return fmt.Errorf("wrapped transaction: %w", err)
				}
				return nil
			}}, nil)

			err = r.Run(t.Context(), func(ctx context.Context) error {
				require.NoError(t, c.WithTransaction(ctx, func(context.Context) error { return nil }))
				postgres.AfterCommit(ctx, func() { panic("observer failed") })
				return r.AfterCommit(ctx, func(context.Context) { effectCalled = true })
			})
			require.ErrorIs(t, err, postgres.ErrPostCommitObserverPanicked)
			require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
			require.Equal(t, 1, fake.Commits())
			require.True(t, wrapperCalled)
			require.True(t, effectCalled)
		})
	}
}

func TestCommitterPropagatesStandaloneObserverPanic(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(t, err)

	observerFailure := errors.New("observer panic")
	require.PanicsWithValue(t, observerFailure, func() {
		_ = c.WithTransaction(t.Context(), func(ctx context.Context) error {
			postgres.AfterCommit(ctx, func() { panic(observerFailure) })
			return nil
		})
	})
	require.Equal(t, 1, fake.Commits())
}

func TestUnitOfWorkDoesNotApplyAfterIndependentCommit(t *testing.T) {
	t.Parallel()
	db, outerDB := testhelpers.NewFakeSQL(t, nil)
	outerCommitter, err := postgres.NewCommitter(db)
	require.NoError(t, err)
	otherDB, innerDB := testhelpers.NewFakeSQL(t, nil)
	innerCommitter, err := postgres.NewCommitter(otherDB)
	require.NoError(t, err)
	inner := uow.New(innerCommitter, nil)

	outer := uow.New(committerWrapper{outerCommitter, func(ctx context.Context, fn func(context.Context) error) error {
		return outerCommitter.WithTransaction(ctx, func(tx context.Context) error {
			if err := fn(tx); err != nil {
				return err
			}
			return inner.Run(t.Context(), func(independent context.Context) error {
				postgres.AfterCommit(independent, func() { panic("observer failed") })
				return nil
			})
		})
	}}, nil)
	var applied bool
	err = outer.Run(t.Context(), func(ctx context.Context) error {
		return uow.OnCommit(ctx, uow.Effect{Apply: func(context.Context) error {
			applied = true
			return nil
		}})
	})
	require.ErrorIs(t, err, postgres.ErrPostCommitObserverPanicked)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.Equal(t, 1, innerDB.Commits())
	require.Equal(t, 1, outerDB.Rollbacks())
	require.Zero(t, outerDB.Commits())
	require.False(t, applied)
}
