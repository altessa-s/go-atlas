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
