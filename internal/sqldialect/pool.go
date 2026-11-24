// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
)

// Pool holds the application pool. PostgreSQL statements may retry once when
// the driver guarantees that the server could not have executed the query.
// BeginTx is delegated directly to database/sql.
type Pool struct {
	*sql.DB
	postgres bool
}

// WrapPool uses the application pool for this SQL dialect.
func (s Style) WrapPool(db *sql.DB) Pool { return Pool{DB: db, postgres: s == Postgres} }

// ExecContext runs a standalone statement. Explicit transactions execute
// their own statements on *sql.Tx.
func (p Pool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
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

// Rows owns the lease until Close. Iteration and scanning errors are final.
type Rows struct {
	*sql.Rows
	conn *sql.Conn
}

// Close releases the rows and their connection.
func (r Rows) Close() error {
	err := r.Rows.Close()
	if r.conn != nil {
		_ = r.conn.Close()
	}
	return err
}

// QueryContext returns rows for the caller to consume and close.
func (p Pool) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	if !p.postgres {
		rows, err := p.DB.QueryContext(ctx, query, args...)
		return Rows{Rows: rows}, err
	}
	rows, conn, err := start(ctx, p.DB, func(c *sql.Conn) (*sql.Rows, error) {
		return c.QueryContext(ctx, query, args...)
	})
	return Rows{Rows: rows, conn: conn}, err
}

// Row owns the lease until Scan.
type Row struct {
	row  *sql.Row
	conn *sql.Conn
	err  error
}

// Scan reads the result and releases its connection.
func (r Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.conn != nil {
		defer func() { _ = r.conn.Close() }()
	}
	return r.row.Scan(dest...)
}

// QueryRowContext examines only the query startup error. Scan retains
// database/sql's single-row, type conversion and no-rows semantics.
func (p Pool) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	if !p.postgres {
		return Row{row: p.DB.QueryRowContext(ctx, query, args...)}
	}
	row, conn, err := start(ctx, p.DB, func(c *sql.Conn) (*sql.Row, error) {
		row := c.QueryRowContext(ctx, query, args...)
		return row, row.Err()
	})
	return Row{row: row, conn: conn, err: err}
}

type retryableError interface {
	error
	SafeToRetry() bool
}

// SafeToRetry reports whether the driver guarantees the statement was never sent.
func SafeToRetry(err error) bool {
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	report, ok := errors.AsType[retryableError](err)
	return ok && report.SafeToRetry()
}

// A statement can consume one replacement connection before returning the
// error to its caller. Explicit transactions never use a replacement.
type retryBudget struct{ used bool }

func (b *retryBudget) take(ctx context.Context, err error) bool {
	if b.used || ctx.Err() != nil || !SafeToRetry(err) {
		return false
	}
	b.used = true
	return true
}

type budgetKey struct{}

// StatementContext gives database/sql a separate retry budget for one call.
func StatementContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, budgetKey{}, new(retryBudget))
}

// DriverError translates a safe statement startup failure to database/sql's
// connection-discard signal. Without a statement budget database/sql governs retries.
func DriverError(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil || !SafeToRetry(err) {
		return err
	}
	if b, ok := ctx.Value(budgetKey{}).(*retryBudget); ok && !b.take(ctx, err) {
		// ErrBadConn would start another attempt, so expose the classification separately.
		return exhausted{err}
	}
	return driver.ErrBadConn
}

type exhausted struct{ error }

func (exhausted) SafeToRetry() bool { return true }

// start holds the connection until the driver has either supplied its result
// or reported a failure starting the statement. pgx can return a SafeToRetry
// error from its first-row preload without translating it to driver.ErrBadConn.
func start[T any](ctx context.Context, db *sql.DB, statement func(*sql.Conn) (T, error)) (T, *sql.Conn, error) {
	var budget retryBudget
	for {
		conn, err := db.Conn(ctx)
		if err != nil {
			var zero T
			return zero, nil, err
		}
		result, err := statement(conn)
		if err == nil {
			return result, conn, nil
		}
		if !errors.Is(err, driver.ErrBadConn) && SafeToRetry(err) {
			// Returning ErrBadConn through Raw discards this exact lease.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
		if !budget.take(ctx, err) {
			return result, nil, err
		}
	}
}
