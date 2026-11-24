// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
)

// Pool holds the application pool. PostgreSQL statements may retry once when
// the driver guarantees that the server could not have executed the query.
// BeginTx is delegated directly to database/sql.
type Pool struct {
	*sql.DB
	postgres bool
	resolve  func(context.Context) (*sql.Tx, bool)
}

// WrapPool uses the application pool for this SQL dialect.
func (s Style) WrapPool(db *sql.DB) Pool { return Pool{DB: db, postgres: s == Postgres} }

// WithResolver returns a pool routing statements through the transaction supplied per call.
func (p Pool) WithResolver(resolve func(context.Context) (*sql.Tx, bool)) Pool {
	p.resolve = resolve
	return p
}

// Tx resolves a caller-owned transaction, if configured.
func (p Pool) Tx(ctx context.Context) (*sql.Tx, bool) {
	if p.resolve != nil {
		if tx, ok := p.resolve(ctx); ok && tx != nil {
			return tx, true
		}
	}
	return nil, false
}

// Scope represents a local or caller-owned transaction for a multi-statement store operation.
type Scope struct {
	*sql.Tx
	owned bool
}

// BeginScope joins the caller's transaction or begins a new one.
func (p Pool) BeginScope(ctx context.Context, opts *sql.TxOptions) (Scope, error) {
	if tx, ok := p.Tx(ctx); ok {
		return Scope{Tx: tx}, nil
	}
	tx, err := p.BeginTx(ctx, opts)
	return Scope{Tx: tx, owned: true}, err
}

// Commit commits a locally owned transaction.
func (t Scope) Commit() error {
	if t.owned {
		return t.Tx.Commit()
	}
	return nil
}

// Rollback rolls back a locally owned transaction.
func (t Scope) Rollback() error {
	if t.owned {
		return t.Tx.Rollback()
	}
	return nil
}

// ExecContext runs a standalone statement. Explicit transactions execute
// their own statements on *sql.Tx.
func (p Pool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if tx, ok := p.Tx(ctx); ok {
		return tx.ExecContext(ctx, query, args...)
	}
	if !p.postgres {
		return p.DB.ExecContext(ctx, query, args...)
	}
	result, conn, err := start(ctx, p.DB, func(c *sql.Conn) (sql.Result, error) {
		return c.ExecContext(ctx, query, args...)
	})
	if conn != nil {
		_ = conn.Close()
	}
	return result, err
}

// Rows owns the lease until exhaustion, cancellation, or Close. Iteration and scanning errors are final.
type Rows struct {
	*sql.Rows
	release func()
}

// Next releases the lease when database/sql closes the final result set.
func (r Rows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	if r.release != nil {
		// Between result sets Next also returns false, but Columns still succeeds.
		if _, err := r.Columns(); err != nil {
			r.release()
		}
	}
	return false
}

// NextResultSet releases the lease when no further result set is available.
func (r Rows) NextResultSet() bool {
	if r.Rows.NextResultSet() {
		return true
	}
	if r.release != nil {
		r.release()
	}
	return false
}

// Close releases the rows and their connection.
func (r Rows) Close() error {
	err := r.Rows.Close()
	if r.release != nil {
		r.release()
	}
	return err
}

// QueryContext returns rows for the caller to consume and close.
func (p Pool) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	if tx, ok := p.Tx(ctx); ok {
		rows, err := tx.QueryContext(ctx, query, args...)
		return Rows{Rows: rows}, err
	}
	if !p.postgres {
		rows, err := p.DB.QueryContext(ctx, query, args...)
		return Rows{Rows: rows}, err
	}
	rows, conn, err := start(ctx, p.DB, func(c *sql.Conn) (*sql.Rows, error) {
		return c.QueryContext(ctx, query, args...)
	})
	return Rows{Rows: rows, release: releaseOnCancel(ctx, conn)}, err
}

// Row owns the lease until Scan.
type Row struct {
	row     *sql.Row
	release func()
	err     error
}

// Scan reads the result and releases its connection.
func (r Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.release != nil {
		defer r.release()
	}
	return r.row.Scan(dest...)
}

// QueryRowContext examines only the query startup error. Scan retains
// database/sql's single-row, type conversion and no-rows semantics.
func (p Pool) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	if tx, ok := p.Tx(ctx); ok {
		return Row{row: tx.QueryRowContext(ctx, query, args...)}
	}
	if !p.postgres {
		return Row{row: p.DB.QueryRowContext(ctx, query, args...)}
	}
	row, conn, err := start(ctx, p.DB, func(c *sql.Conn) (*sql.Row, error) {
		row := c.QueryRowContext(ctx, query, args...)
		return row, row.Err()
	})
	return Row{row: row, release: releaseOnCancel(ctx, conn), err: err}
}

// database/sql closes the result on cancellation. Return its separate pool
// lease as well, even if the caller has not reached Close or Scan yet.
func releaseOnCancel(ctx context.Context, conn *sql.Conn) func() {
	if conn == nil {
		return nil
	}
	closeConn := func() { _ = conn.Close() }
	stop := context.AfterFunc(ctx, closeConn)
	return func() {
		stop()
		closeConn()
	}
}

type retryableError interface {
	error
	SafeToRetry() bool
}

// SafeToRetry reports whether the driver guarantees the statement was never sent.
func SafeToRetry(err error) bool {
	for err != nil {
		switch cause := err.(type) {
		case retryableError:
			return cause.SafeToRetry()
		case interface{ Unwrap() []error }:
			// One pre-send diagnostic cannot vouch for the other failures.
			causes := cause.Unwrap()
			for _, e := range causes {
				if !SafeToRetry(e) {
					return false
				}
			}
			return len(causes) > 0
		case interface{ Unwrap() error }:
			err = cause.Unwrap()
		default:
			return errors.Is(err, driver.ErrBadConn)
		}
	}
	return false
}

// A statement can consume one replacement connection before returning the
// error to its caller. Explicit transactions never use a replacement.
type retryBudget struct{ used atomic.Bool }

func (b *retryBudget) take(ctx context.Context, err error) bool {
	return ctx.Err() == nil && SafeToRetry(err) && b.used.CompareAndSwap(false, true)
}

type budgetKey struct{}

// StatementContext gives database/sql a separate retry budget for one call.
func StatementContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, budgetKey{}, new(retryBudget))
}

// DriverError translates a safe statement startup failure to database/sql's
// connection-discard signal. Without a statement budget database/sql governs retries.
func DriverError(ctx context.Context, err error) error {
	if !SafeToRetry(err) {
		if errors.Is(err, driver.ErrBadConn) {
			// database/sql would otherwise replay a mixed failure on its own.
			return ambiguous{err}
		}
		return err
	}
	if ctx.Err() != nil {
		return err
	}
	if b, ok := ctx.Value(budgetKey{}).(*retryBudget); ok && !b.take(ctx, err) {
		// Only ErrBadConn needs masking to stop database/sql from retrying.
		if errors.Is(err, driver.ErrBadConn) {
			return exhausted{err}
		}
		return err
	}
	return driver.ErrBadConn
}

// Hide only the replay signal while retaining unrelated diagnostics.
type ambiguous struct{ error }

func (e ambiguous) Is(target error) bool {
	return target != driver.ErrBadConn && errors.Is(e.error, target)
}

func (e ambiguous) As(target any) bool { return errors.As(e.error, target) }
func (ambiguous) SafeToRetry() bool    { return false }

type exhausted struct{ error }

func (exhausted) SafeToRetry() bool { return true }

// start holds the connection until the driver has either supplied its result
// or reported a failure starting the statement. pgx can return a SafeToRetry
// error from its first-row preload without translating it to driver.ErrBadConn.
func start[T any](ctx context.Context, db *sql.DB, statement func(*sql.Conn) (T, error)) (T, *sql.Conn, error) {
	for attempt := 0; ; attempt++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			var zero T
			return zero, nil, err
		}
		result, err := statement(conn)
		if err == nil {
			return result, conn, nil
		}
		safe := SafeToRetry(err)
		if safe && !errors.Is(err, driver.ErrBadConn) {
			// Returning ErrBadConn through Raw discards this exact lease.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
		if ctxErr := ctx.Err(); safe && ctxErr != nil {
			return result, nil, ctxErr
		}
		if !safe || attempt > 0 {
			return result, nil, err
		}
	}
}
