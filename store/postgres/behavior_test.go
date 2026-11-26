// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		code, msg string
		kind      ErrorKind
	}{
		{"23505", "", Unique}, {"23503", "", ForeignKey}, {"23514", "", Check}, {"40001", "", Serialization}, {"40P01", "", Deadlock},
		{"57014", "canceling statement due to statement timeout", StatementTimeout}, {"57014", "canceling statement due to user request", Canceled},
		{"57014", "unrecognized cancellation diagnostic", Unknown}, {"08006", "", ConnectionLost}, {"57P01", "", ConnectionLost}, {"42P01", "", Unknown},
	} {
		e := fmt.Errorf("execute: %w", &pgconn.PgError{Code: tc.code, Message: tc.msg, ConstraintName: "items_constraint"})
		k, name := Classify(e)
		require.Equal(t, tc.kind, k)
		if k == Unique || k == Check || k == ForeignKey {
			require.Equal(t, "items_constraint", name)
		} else {
			require.Empty(t, name)
		}
	}
	for err, want := range map[error]ErrorKind{nil: Unknown, context.Canceled: Canceled, context.DeadlineExceeded: Canceled,
		io.ErrUnexpectedEOF: ConnectionLost, driver.ErrBadConn: ConnectionBeforeSend} {
		kind, _ := Classify(err)
		require.Equal(t, want, kind)
	}
	missing, duplicate := errors.New("missing"), errors.New("duplicate")
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "item_pkey"}
	require.ErrorIs(t, ResourceError(unique, missing, duplicate), unique)
	require.ErrorIs(t, ResourceError(unique, missing, duplicate), duplicate)
	require.ErrorIs(t, ResourceError(sql.ErrNoRows, missing, duplicate), missing)
	require.NoError(t, ResourceError(nil, missing, duplicate))
	require.ErrorIs(t, Missing(sql.ErrNoRows, missing), missing)
	require.ErrorIs(t, Changed(0, nil, missing), missing)
	require.NoError(t, Changed(1, nil, missing))
	require.ErrorIs(t, Changed(0, io.EOF, missing), io.EOF)
}

func TestValues(t *testing.T) {
	for _, d := range []time.Duration{0, time.Microsecond, 24 * time.Hour, time.Duration(math.MaxInt64).Truncate(time.Microsecond)} {
		i, err := Interval(d)
		require.NoError(t, err)
		got, err := Duration(i)
		require.NoError(t, err)
		require.Equal(t, d, got)
	}
	_, err := Interval(time.Nanosecond)
	require.Error(t, err)
	for _, i := range []pgtype.Interval{
		{}, {Valid: true, Months: 1}, {Valid: true, Days: -1}, {Valid: true, Days: math.MaxInt32},
		{Valid: true, Microseconds: time.Duration(math.MaxInt64).Microseconds() + 1},
		{Valid: true, Microseconds: math.MaxInt64}, {Valid: true, Microseconds: math.MinInt64},
	} {
		_, err := Duration(i)
		require.Error(t, err)
	}
	got, err := Duration(pgtype.Interval{Valid: true, Days: 1, Microseconds: 1})
	require.NoError(t, err)
	require.Equal(t, 24*time.Hour+time.Microsecond, got)
	for _, interval := range []pgtype.Interval{
		{Valid: true, Days: 1, Microseconds: -time.Hour.Microseconds()},
		{Valid: true, Days: -1, Microseconds: (47 * time.Hour).Microseconds()},
	} {
		got, err = Duration(interval)
		require.NoError(t, err)
		require.Equal(t, 23*time.Hour, got)
	}
	for _, n := range []int{0, -1} {
		_, err := BatchSize(n)
		require.Error(t, err)
	}
	n, err := BatchSize(math.MaxInt32)
	require.NoError(t, err)
	require.EqualValues(t, math.MaxInt32, n)
}

func TestPoolRoutingAndHooks(t *testing.T) {
	db, primary := testhelpers.NewFakeSQL(t, nil)
	reserved, worker := testhelpers.NewFakeSQL(t, nil)
	ctx := WithPool(t.Context(), db, reserved)
	q := Querier(db)
	wake := make(chan struct{}, 2)
	called := 0
	AfterCommit(ctx, func() { called++ })
	require.NoError(t, WithSavepoint(ctx, db, func(txCtx context.Context) error {
		require.True(t, InWriteTransaction(txCtx))
		require.NoError(t, CheckDatabase(txCtx, db))
		_, err := q.ExecContext(txCtx, "UPDATE items SET n = 1")
		require.NoError(t, err)
		NotifyAfterCommit(txCtx, wake)
		NotifyAfterCommit(txCtx, wake)
		require.Empty(t, wake)
		AfterCommitContext(txCtx, func(fresh context.Context) {
			tx, ok, _ := Tx(fresh)
			require.False(t, ok)
			require.Nil(t, tx)
			called++
		})
		return nil
	}))
	require.Equal(t, 2, called)
	require.Len(t, wake, 1)
	require.Empty(t, primary.Calls())
	require.True(t, worker.Calls()[0].InTx)
	require.Equal(t, health.StatusServing, Checker(db).CheckHealth(ctx))
	p := Pools{"main": db, "worker": reserved}
	require.NoError(t, p.Close())
	require.Equal(t, health.StatusNotServing, Checker(db).CheckHealth(ctx))
}

func TestOpenRejectsInvalidInputs(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		option  Option
		message string
	}{
		{WithPooling("unknown"), "unknown PostgreSQL pooling mode"},
		{WithMaxOpenConnections(0), "invalid PostgreSQL pool limits"},
		{WithMaxIdleConnections(-1), "invalid PostgreSQL pool limits"},
		{WithStatementTimeout(time.Nanosecond), "statement_timeout must be between"},
		{WithStatementTimeout(-1), "statement_timeout must be between"},
		{WithConnectionMaxLifetime(0), "invalid PostgreSQL pool limits"},
	} {
		_, err := Open(ctx, "postgres://localhost/db", tc.option)
		require.ErrorContains(t, err, tc.message)
	}
	for _, dsn := range []string{"", "postgres://hidden:password@%gh/db"} {
		_, err := NewConnector(redacted.RedactedString(dsn))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
	}
}

func TestTransactionCommitFailuresDiscardHooks(t *testing.T) {
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cause := errors.New("discard")
	fn := func() { t.Error("hook ran after rollback") }
	err := WithTransaction(t.Context(), db, func(ctx context.Context) error {
		AfterCommit(ctx, fn)
		return WithSavepoint(ctx, db, func(child context.Context) error { AfterCommit(child, fn); return cause })
	})
	require.ErrorIs(t, err, cause)
}

func TestPooledDefaults(t *testing.T) {
	for _, state := range []string{"0", "10s"} {
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
			return testhelpers.FakeSQLReply{Columns: []string{"statement", "lock", "idle"}, Rows: [][]driver.Value{{state, "2s", "5s"}}}
		})
		err := checkPooledDefaults(t.Context(), db)
		if state == "0" {
			require.ErrorIs(t, err, ErrPooledTimeoutDefaults)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestConnSettingsAndUnsafeStartup(t *testing.T) {
	require.Equal(t, "transaction", PoolingTransaction.String())
	for _, dsn := range []string{
		"postgres://localhost/app?options=-csearch_path%3Dother",
		"postgres://localhost/app?default_query_exec_mode=simple_protocol",
		"postgres://localhost/app?default_query_exec_mode=describe_exec",
	} {
		_, err := NewConnector(redacted.RedactedString(dsn), WithPooling(PoolingTransaction))
		require.Error(t, err)
	}
	ctx := t.Context()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := WithTransactionSettings(ctx, db, TransactionSettings{SynchronousCommit: "unknown"})
	require.Error(t, err)
	_, err = WithTransactionSettings(ctx, db, TransactionSettings{LockTimeout: -1})
	require.Error(t, err)
	same, err := WithTransactionSettings(ctx, db, TransactionSettings{})
	require.NoError(t, err)
	require.Same(t, ctx, same)
	_, err = NewConnector("postgres://localhost/app", WithTracer(nil))
	require.NoError(t, err)
}

func TestSavepointPanicCanBeRecoveredByOwner(t *testing.T) {
	db, fake := testhelpers.NewFakeSQL(t, nil)
	require.NoError(t, WithTransaction(t.Context(), db, func(ctx context.Context) error {
		require.Panics(t, func() {
			_ = WithSavepoint(ctx, db, func(child context.Context) error {
				AfterCommit(child, func() { t.Fatal("discarded hook ran") })
				panic("handler failed")
			})
		})
		_, err := Queryer(ctx, db).ExecContext(ctx, "SELECT 1")
		return err
	}))
	require.Equal(t, 1, fake.Commits())
	require.Contains(t, fake.Calls()[1].Query, "ROLLBACK TO SAVEPOINT")
}

func TestPoolBinding(t *testing.T) {
	sameIdentity := func(query string, _ []any) testhelpers.FakeSQLReply {
		if query == controlIdentityQuery {
			return testhelpers.FakeSQLReply{
				Columns: []string{"system_identifier", "current_database"},
				Rows:    [][]driver.Value{{"1", "app"}},
			}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	}
	primary, _ := testhelpers.NewFakeSQL(t, sameIdentity)
	worker, _ := testhelpers.NewFakeSQL(t, sameIdentity)
	other, _ := testhelpers.NewFakeSQL(t, nil)
	pools := Pools{DefaultPool: primary, "worker": worker}
	require.NoError(t, pools.Verify(t.Context()))
	db, resolve, err := pools.Binding(t.Context(), "worker")
	require.NoError(t, err)
	require.Same(t, worker, db)
	db, _, err = pools.Binding(t.Context(), "missing")
	require.Error(t, err)
	require.Nil(t, db)
	_, ok, err := resolve(t.Context())
	require.NoError(t, err)
	require.False(t, ok)
	for _, source := range []*sql.DB{primary, worker, other} {
		require.NoError(t, WithTransaction(t.Context(), source, func(ctx context.Context) error {
			tx, bound, err := resolve(ctx)
			require.NoError(t, err)
			require.Equal(t, source != other, bound)
			if bound {
				current, _, _ := Tx(ctx)
				require.Same(t, current, tx)
			}
			return nil
		}))
	}
}

func TestRetryableTransaction(t *testing.T) {
	for _, kind := range []ErrorKind{Unique, ForeignKey, Check, Serialization, Deadlock, ConnectionBeforeSend, ConnectionLost, Canceled, Timeout, StatementTimeout, Unknown} {
		require.Equal(t, kind == Serialization || kind == Deadlock, kind.RetryableTransaction(), string(kind))
	}
}

func TestExecutorPreparedAndRow(t *testing.T) {
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"n"}, Rows: [][]driver.Value{{int64(7)}}}
	})
	run := func(ctx context.Context) error {
		q := Querier(db)
		stmt, err := q.PrepareContext(ctx, "UPDATE items SET n=$1")
		if err != nil {
			return err
		}
		defer func() { _ = stmt.Close() }()
		if _, err = stmt.ExecContext(ctx, 7); err != nil {
			return err
		}
		var n int
		err = q.QueryRowContext(ctx, "SELECT n FROM items").Scan(&n)
		require.Equal(t, 7, n)
		return err
	}
	require.NoError(t, run(t.Context()))
	require.NoError(t, WithTransaction(t.Context(), db, run))
	for i, call := range fake.Calls() {
		require.Equal(t, i >= 2, call.InTx)
	}
}

func TestPoolOptions(t *testing.T) {
	m := metrics.New()
	logger := slog.New(slog.DiscardHandler)
	o := newOptions(WithMeter(m), WithLogger(logger), WithPoolName("jobs"), WithConnectTimeout(2*time.Second), WithConnectionMaxIdleTime(time.Minute))
	require.Same(t, m, o.meter)
	require.Same(t, logger, o.logger)
	require.Equal(t, "jobs", o.poolName)
	require.Equal(t, 2*time.Second, o.connectTimeout)
	require.Equal(t, time.Minute, o.connectionMaxIdleTime)
}

func TestBeginAsyncCommitIsIndependent(t *testing.T) {
	db, fake := testhelpers.NewFakeSQL(t, nil)
	require.NoError(t, WithTransaction(t.Context(), db, func(ctx context.Context) error {
		async, err := BeginAsyncCommit(ctx, db)
		require.NoError(t, err)
		outer, _, _ := Tx(ctx)
		require.NotSame(t, outer, async)
		require.NoError(t, async.Rollback())
		return nil
	}))
	require.Equal(t, 1, fake.Commits())
	require.Equal(t, 1, fake.Rollbacks())
}

func TestFatalConnectionError(t *testing.T) {
	err := &pgconn.PgError{Severity: "FATAL", Code: "25P03", Message: "idle transaction timeout"}
	kind, _ := Classify(err)
	require.Equal(t, ConnectionLost, kind)
}

func TestCommitHookPanicPreservesOtherHooks(t *testing.T) {
	db, fake := testhelpers.NewFakeSQL(t, nil)
	wake := make(chan struct{}, 1)
	var calls []int
	require.PanicsWithValue(t, "observer", func() {
		_ = WithTransaction(t.Context(), db, func(ctx context.Context) error {
			AfterCommit(ctx, func() { calls = append(calls, 1); panic("observer") })
			AfterCommit(ctx, func() { calls = append(calls, 2) })
			NotifyAfterCommit(ctx, wake)
			return nil
		})
	})
	require.Equal(t, 1, fake.Commits())
	require.Equal(t, []int{1, 2}, calls)
	require.Len(t, wake, 1)
}

func TestLocalizedFatalConnectionError(t *testing.T) {
	err := &pgconn.PgError{Severity: "FATALE", SeverityUnlocalized: "FATAL", Code: "25P03"}
	kind, _ := Classify(err)
	require.Equal(t, ConnectionLost, kind)
}

func TestPoolBinding_RejectsMismatchedDatabase(t *testing.T) {
	identityResponder := func(sysID, dbName string) func(string, []any) testhelpers.FakeSQLReply {
		return func(query string, _ []any) testhelpers.FakeSQLReply {
			if query == "SELECT system_identifier::text, current_database() FROM pg_control_system()" {
				return testhelpers.FakeSQLReply{
					Columns: []string{"system_identifier", "current_database"},
					Rows:    [][]driver.Value{{sysID, dbName}},
				}
			}
			return testhelpers.FakeSQLReply{Affected: 1}
		}
	}

	ordersDB, ordersFake := testhelpers.NewFakeSQL(t, identityResponder("100", "orders"))
	reportingDB, reportingFake := testhelpers.NewFakeSQL(t, identityResponder("100", "reporting"))
	pools := Pools{DefaultPool: ordersDB, "reporting": reportingDB}

	boundDB, resolve, err := pools.Binding(t.Context(), "reporting")
	require.NoError(t, err)
	boundPool := sqldialect.Postgres.WrapPool(boundDB).WithResolver(resolve)

	err = WithTransaction(t.Context(), ordersDB, func(ctx context.Context) error {
		tx, open, err := boundPool.Tx(ctx)
		require.Nil(t, tx)
		require.False(t, open)
		require.Equal(t, ErrDatabaseMismatch, err)
		_, err = boundPool.BeginScope(ctx, nil)
		require.Equal(t, ErrDatabaseMismatch, err)
		_, err = boundPool.QueryContext(ctx, "SELECT 1")
		require.Equal(t, ErrDatabaseMismatch, err)
		require.Equal(t, ErrDatabaseMismatch, boundPool.QueryRowContext(ctx, "SELECT 1").Scan())
		before := sqlTransactionGoroutines()
		for range 128 {
			_, execErr := boundPool.ExecContext(ctx, "INSERT INTO events_outbox (id) VALUES ($1)", "e1")
			require.Equal(t, ErrDatabaseMismatch, execErr)
		}
		require.Equal(t, before, sqlTransactionGoroutines(), "rejected calls must not leave database/sql transaction goroutines")
		return ErrDatabaseMismatch
	})
	require.ErrorIs(t, err, ErrDatabaseMismatch)
	require.ErrorIs(t, pools.Verify(t.Context()), ErrDatabaseMismatch)
	for _, call := range ordersFake.Calls() {
		require.NotContains(t, call.Query, "INSERT INTO events_outbox")
	}
	for _, call := range reportingFake.Calls() {
		require.NotContains(t, call.Query, "INSERT INTO events_outbox")
	}
}

func sqlTransactionGoroutines() int {
	for size := 64 << 10; ; size *= 2 {
		buf := make([]byte, size)
		if n := runtime.Stack(buf, true); n < size {
			return strings.Count(string(buf[:n]), "created by database/sql.(*DB).beginDC in goroutine")
		}
	}
}

func TestPoolBinding_SingleConnectionWithoutVerify(t *testing.T) {
	t.Parallel()
	identity := func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{
			Columns: []string{"system_identifier", "current_database"},
			Rows:    [][]driver.Value{{"100", "orders"}},
		}
	}
	source, _ := testhelpers.NewFakeSQL(t, identity)
	source.SetMaxOpenConns(1)
	target, _ := testhelpers.NewFakeSQL(t, identity)
	db, resolve, err := (Pools{"source": source, "target": target}).Binding(t.Context(), "target")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, WithTransaction(ctx, source, func(txCtx context.Context) error {
		_, err := sqldialect.Postgres.WrapPool(db).WithResolver(resolve).ExecContext(txCtx, "UPDATE orders SET status = 'ready'")
		return err
	}))
}

func TestPoolIdentity_WaitRespectsContext(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		close(started)
		<-release
		return testhelpers.FakeSQLReply{
			Columns: []string{"system_identifier", "current_database"},
			Rows:    [][]driver.Value{{"100", "orders"}},
		}
	})
	first := make(chan error, 1)
	go func() {
		_, err := resolvePoolIdentity(t.Context(), db)
		first <- err
	}()
	<-started
	defer func() {
		close(release)
		require.NoError(t, <-first)
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	waiter := make(chan error, 1)
	go func() {
		_, err := resolvePoolIdentity(ctx, db)
		waiter <- err
	}()
	select {
	case err := <-waiter:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("identity lookup waited for stalled I/O after its context expired")
	}
}

func TestPoolIdentity_CachesFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("identity unavailable")
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: failure}
	})
	for range 2 {
		_, err := resolvePoolIdentity(t.Context(), db)
		require.ErrorIs(t, err, failure)
	}
	require.Len(t, fake.Calls(), 1)
}

func TestPoolIdentity_FallbackWhenControlSystemUnreadable(t *testing.T) {
	t.Parallel()

	restrictedResponder := func(dbName string) func(string, []any) testhelpers.FakeSQLReply {
		return func(query string, _ []any) testhelpers.FakeSQLReply {
			switch query {
			case controlIdentityQuery:
				return testhelpers.FakeSQLReply{Err: &pgconn.PgError{Code: "42501", Message: "permission denied for function pg_control_system"}}
			case databaseIdentityQuery:
				return testhelpers.FakeSQLReply{
					Columns: []string{"current_database"},
					Rows:    [][]driver.Value{{dbName}},
				}
			default:
				return testhelpers.FakeSQLReply{Affected: 1}
			}
		}
	}

	var logRecords int
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_ = logger
	h := &countingHandler{}
	db1, _ := testhelpers.NewFakeSQL(t, restrictedResponder("app"))
	registerPoolLogger(db1, slog.New(h))
	db2, _ := testhelpers.NewFakeSQL(t, restrictedResponder("app"))
	registerPoolLogger(db2, slog.New(h))

	pools := Pools{DefaultPool: db1, "worker": db2}
	require.NoError(t, pools.Verify(t.Context()))
	require.NoError(t, pools.Verify(t.Context()))
	_ = logRecords
	require.Equal(t, 2, h.count)
}

type countingHandler struct{ count int }

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *countingHandler) WithGroup(string) slog.Handler            { return h }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelInfo {
		h.count++
	}
	return nil
}

func TestClassifyContext(t *testing.T) {
	t.Parallel()

	activeCtx := t.Context()
	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	deadlineCtx, expire := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer expire()

	translated := &pgconn.PgError{Code: "57014", Message: "annulation de la commande à cause du délai d'attente"}
	englishStmtTimeout := &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}
	englishUserCancel := &pgconn.PgError{Code: "57014", Message: "canceling statement due to user request"}
	safe57014 := sqldialect.SafeError(nil, "query", translated)

	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want ErrorKind
	}{
		{name: "deadline ctx with translated 57014", ctx: deadlineCtx, err: translated, want: Timeout},
		{name: "deadline ctx with english user cancel 57014", ctx: deadlineCtx, err: englishUserCancel, want: Timeout},
		{name: "deadline ctx with safe 57014", ctx: deadlineCtx, err: safe57014, want: Timeout},
		{name: "deadline error directly", ctx: activeCtx, err: context.DeadlineExceeded, want: Timeout},
		{name: "canceled ctx with translated 57014", ctx: canceledCtx, err: translated, want: Canceled},
		{name: "canceled ctx with safe 57014", ctx: canceledCtx, err: safe57014, want: Canceled},
		{name: "canceled error directly", ctx: activeCtx, err: context.Canceled, want: Canceled},
		{name: "no ctx error with translated 57014", ctx: activeCtx, err: translated, want: StatementTimeout},
		{name: "no ctx error with safe 57014", ctx: activeCtx, err: safe57014, want: StatementTimeout},
		{name: "no ctx error with english statement timeout", ctx: activeCtx, err: englishStmtTimeout, want: StatementTimeout},
		{name: "no ctx error with english user request", ctx: activeCtx, err: englishUserCancel, want: StatementTimeout},
		{name: "nil ctx with translated 57014", ctx: nil, err: translated, want: StatementTimeout},
		{name: "nil error with deadline ctx", ctx: deadlineCtx, err: nil, want: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kind, name := ClassifyContext(tc.ctx, tc.err)
			require.Equal(t, tc.want, kind)
			require.Empty(t, name)
		})
	}

	// Guard: Classify without context keeps existing outcomes.
	kind, _ := Classify(translated)
	require.Equal(t, Unknown, kind)
	kind, _ = Classify(englishStmtTimeout)
	require.Equal(t, StatementTimeout, kind)
	kind, _ = Classify(englishUserCancel)
	require.Equal(t, Canceled, kind)
}
