// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/altessa-s/go-atlas/internal/sqldialect"
)

// DBTX is the database/sql surface shared by a pool and a transaction.
type DBTX interface {
	// ExecContext executes a statement and returns its result.
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	// PrepareContext prepares a statement on the selected pool or transaction.
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	// QueryContext opens a result set. The caller closes the rows.
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	// QueryRowContext returns a single row whose Scan reports query errors.
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Tx returns the attached transaction and whether it is still open. A finished transaction
// returns its handle with false so statement resolvers can fail closed. Only the owner may end it.
func Tx(ctx context.Context) (*sql.Tx, bool) {
	t := attached(ctx)
	if t == nil {
		return nil, false
	}
	return t.tx, t.open()
}

func attached(ctx context.Context) *transaction {
	t, _ := ctx.Value(transactionKey{}).(*transaction)
	return t
}

func (t *transaction) open() bool {
	return !t.life.done.Load() && t.ownerContext.Err() == nil
}

// InWriteTransaction reports whether ctx belongs to an open writable transaction.
func InWriteTransaction(ctx context.Context) bool {
	t := attached(ctx)
	return t != nil && !t.readOnly && t.open()
}

// NotifyAfterCommit sends a coalescing wake-up once the enclosing transaction commits.
// Without an enclosing transaction the write is already visible.
func NotifyAfterCommit(ctx context.Context, wake chan<- struct{}) {
	if t, ok := ctx.Value(transactionKey{}).(*transaction); ok {
		t.mu.Lock()
		if t.notifications == nil {
			t.notifications = make(map[chan<- struct{}]struct{})
		}
		t.notifications[wake] = struct{}{}
		t.mu.Unlock()
		return
	}
	notify(wake)
}

// AfterCommit runs observe after the outer transaction commits. Callbacks run synchronously in registration order.
// Outside a transaction it runs observe immediately.
func AfterCommit(ctx context.Context, observe func()) {
	if t, ok := ctx.Value(transactionKey{}).(*transaction); ok {
		t.mu.Lock()
		t.observers = append(t.observers, observe)
		t.mu.Unlock()
		return
	}
	observe()
}

// WithPool routes operations on db through another pool connected to the same database.
// Nested repositories retain the selected connection when they join a transaction.
func WithPool(ctx context.Context, db, pool *sql.DB) context.Context {
	return context.WithValue(ctx, poolKey{}, poolRoute{db: db, pool: pool})
}

// CheckDatabase ensures that a transaction-bound operation uses its own connection pool.
func CheckDatabase(ctx context.Context, db *sql.DB) error {
	if current, ok := ctx.Value(transactionKey{}).(*transaction); ok && current.db != selectedPool(ctx, db) {
		return ErrDifferentDatabase
	}
	return nil
}

// Queryer returns the transaction attached to ctx, or db outside a transaction.
func Queryer(ctx context.Context, db *sql.DB) DBTX {
	if t := attached(ctx); t != nil {
		if t.open() {
			return t.tx
		}
		return Executor{bound: t}
	}
	return Executor{db: selectedPool(ctx, db)}
}

// Querier returns a DBTX that resolves [Queryer] on every call. It serves components built once
// over the pool, such as list engines, whose calls may run inside a transaction.
func Querier(db *sql.DB) Executor {
	return Executor{db: db}
}

// WithTransaction runs handler in one transaction.
// Nested calls reuse the transaction. Its owner commits on success and rolls back on error or panic.
func WithTransaction(ctx context.Context, db *sql.DB, handler func(context.Context) error) error {
	return withTransaction(ctx, db, nil, handler)
}

// WithSnapshot starts a read-only repeatable-read transaction for consistent multi-query reads.
// Inside an existing transaction it joins the owner and inherits its isolation and access mode.
func WithSnapshot(ctx context.Context, db *sql.DB, handler func(context.Context) error) error {
	return withTransaction(
		ctx,
		db,
		&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true},
		handler,
	)
}

// target resolves a call before opening its result set. Only a standalone
// statement receives a connection replacement budget.
func (q Executor) target(ctx context.Context) (context.Context, DBTX) {
	t := q.bound
	if t == nil {
		t = attached(ctx)
	}
	if t != nil {
		if !t.open() {
			// database/sql checks the statement context before checking ErrTxDone.
			if t.ownerContext.Err() != nil {
				_ = t.tx.Rollback()
			}
			return context.WithoutCancel(ctx), t.tx
		}
		return ctx, t.tx
	}
	return sqldialect.StatementContext(ctx), selectedPool(ctx, q.db)
}

// ExecContext routes the operation to the current transaction or pool.
func (q Executor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, target := q.target(ctx)
	return target.ExecContext(ctx, query, args...)
}

// PrepareContext routes the operation to the current transaction or pool.
func (q Executor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	ctx, target := q.target(ctx)
	return target.PrepareContext(ctx, query)
}

// QueryContext routes the operation to the current transaction or pool.
func (q Executor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, target := q.target(ctx)
	return target.QueryContext(ctx, query, args...)
}

// QueryRowContext routes the operation to the current transaction or pool.
func (q Executor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, target := q.target(ctx)
	return target.QueryRowContext(ctx, query, args...)
}

type poolKey struct{}
type poolRoute struct{ db, pool *sql.DB }

func selectedPool(ctx context.Context, db *sql.DB) *sql.DB {
	if route, ok := ctx.Value(poolKey{}).(poolRoute); ok && route.db == db {
		return route.pool
	}
	return db
}

type transactionKey struct{}

type transactionLife struct {
	done     atomic.Bool
	poisoned atomic.Bool
	replay   bool
}

type transaction struct {
	life          *transactionLife
	db            *sql.DB
	tx            *sql.Tx
	readOnly      bool
	ownerContext  context.Context
	mu            sync.Mutex
	notifications map[chan<- struct{}]struct{}
	observers     []func()
}

// Executor routes database/sql calls through the transaction attached to each context. Construct it with [Querier].
type Executor struct {
	bound *transaction
	db    *sql.DB
}

func withTransaction(
	ctx context.Context,
	db *sql.DB,
	options *sql.TxOptions,
	handler func(context.Context) error,
) (err error) {
	if current := attached(ctx); current != nil {
		if !current.open() {
			return sql.ErrTxDone
		}
		if err = CheckDatabase(ctx, db); err != nil {
			return err
		}
		defer func() {
			if r := recover(); r != nil {
				current.life.poisoned.Store(true)
				panic(r)
			}
		}()
		return diagnostic("joined transaction body", handler(ctx))
	}

	db = selectedPool(ctx, db)
	tx, err := db.BeginTx(ctx, options)
	if err != nil {
		return diagnostic("begin transaction", err)
	}
	current := &transaction{life: new(transactionLife), db: db, tx: tx, ownerContext: ctx, readOnly: options != nil && options.ReadOnly}
	defer func() {
		defer current.life.done.Store(true)
		if err != nil && ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
			err = errors.Join(err, ctx.Err())
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, diagnostic("rollback transaction", rollbackErr))
		}
	}()

	ctx = context.WithValue(ctx, transactionKey{}, current)

	err = diagnostic("transaction body", handler(ctx))
	if current.life.poisoned.Load() {
		return errors.Join(ErrTransactionPanicked, err)
	}
	if err != nil {
		return err
	}
	if current.life.done.Load() {
		return sql.ErrTxDone
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	err = tx.Commit()
	current.life.done.Store(true)
	if err != nil {
		kind, _ := Classify(err)
		safe := diagnostic("commit transaction", err)
		state := sqldialect.SQLState(err)
		if errors.Is(err, pgx.ErrTxCommitRollback) || (state != "" && state != "40003" && kind != ConnectionLost) || sqldialect.SafeToRetry(err) {
			return safe
		}
		return &uncertainCommit{safe}
	}
	observers, notifications := current.takeHooks()
	// Every hook belongs to an already committed write. Defers preserve delivery
	// if one observer panics, while letting its panic reach the caller.
	for wake := range notifications {
		defer notify(wake)
	}
	for _, observe := range slices.Backward(observers) {
		defer observe()
	}
	return nil
}

func notify(wake chan<- struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (t *transaction) takeHooks() ([]func(), map[chan<- struct{}]struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	observers, notifications := t.observers, t.notifications
	t.observers, t.notifications = nil, nil
	return observers, notifications
}

// AfterCommitContext calls fn with request values, without the deadline, cancellation
// or transaction settings of the completed operation.
func AfterCommitContext(ctx context.Context, fn func(context.Context)) {
	AfterCommit(ctx, func() {
		fresh := context.WithValue(context.WithoutCancel(ctx), transactionKey{}, nil)
		fresh = context.WithValue(fresh, settingsKey{}, "")
		fn(fresh)
	})
}

// WithSavepoint isolates a nested operation and its commit hooks. Without a transaction
// it starts one. Only the handler and its nested calls may use the transaction during the scope.
func WithSavepoint(ctx context.Context, db *sql.DB, handler func(context.Context) error) (err error) {
	defer func() {
		if err != nil {
			err = diagnostic("savepoint", err)
		}
	}()
	parent, ok := ctx.Value(transactionKey{}).(*transaction)
	if !ok {
		return WithTransaction(ctx, db, handler)
	}
	if !parent.open() {
		return sql.ErrTxDone
	}
	if err = CheckDatabase(ctx, db); err != nil {
		return err
	}
	const name = "atlas_nested_operation"
	if _, err = parent.tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	released := false
	defer func() {
		if released {
			return
		}
		if _, rollbackErr := parent.tx.ExecContext(parent.ownerContext, "ROLLBACK TO SAVEPOINT "+name); rollbackErr != nil {
			err = errors.Join(err, rollbackErr, parent.tx.Rollback())
			parent.life.done.Store(true)
			return
		}
		_, releaseErr := parent.tx.ExecContext(parent.ownerContext, "RELEASE SAVEPOINT "+name)
		err = errors.Join(err, releaseErr)
	}()
	child := &transaction{life: parent.life, db: parent.db, ownerContext: parent.ownerContext, tx: parent.tx, readOnly: parent.readOnly}
	if err = handler(context.WithValue(ctx, transactionKey{}, child)); err != nil {
		return err
	}
	if _, err = parent.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return err
	}
	released = true
	observers, wakes := child.takeHooks()
	parent.mu.Lock()
	defer parent.mu.Unlock()
	parent.observers = append(parent.observers, observers...)
	if parent.notifications == nil {
		parent.notifications = wakes
	} else {
		maps.Copy(parent.notifications, wakes)
	}
	return nil
}
