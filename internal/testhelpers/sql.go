// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
)

// FakeSQLReply scripts the answer of a [FakeSQL] database to one statement:
// rows for a query, an affected-row count for an exec, or an error for both.
type FakeSQLReply struct {
	Columns  []string
	Rows     [][]driver.Value
	Affected int64
	Err      error
}

// FakeSQLCall is one statement a [FakeSQL] database received.
type FakeSQLCall struct {
	Query string
	Args  []any
	// InTx reports whether the statement ran inside a transaction.
	InTx bool
}

// FakeSQL is a database/sql driver that records every statement and answers
// from a script, so SQL-building code can be tested for statement shape,
// argument order and result mapping without a server or a third-party driver.
// Prepared statements record each execution like a direct one, and
// transactions only mark the statements issued inside them.
type FakeSQL struct {
	mu        sync.Mutex
	calls     []FakeSQLCall
	closed    int
	commits   int
	rollbacks int
	noRecord  bool
	respond   func(query string, args []any) FakeSQLReply
}

// NewFakeSQL returns a *sql.DB backed by a [FakeSQL] and the fake itself.
// respond may be nil, in which case every statement succeeds affecting one row.
// The database is closed via tb.Cleanup.
func NewFakeSQL(tb testing.TB, respond func(query string, args []any) FakeSQLReply) (*sql.DB, *FakeSQL) {
	tb.Helper()
	f := &FakeSQL{respond: respond}
	if f.respond == nil {
		f.respond = func(string, []any) FakeSQLReply { return FakeSQLReply{Affected: 1} }
	}
	db := sql.OpenDB(f)
	tb.Cleanup(func() { _ = db.Close() })
	return db, f
}

// Calls returns the statements received so far, in order.
func (f *FakeSQL) Calls() []FakeSQLCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeSQLCall(nil), f.calls...)
}

// DisableRecording stops [FakeSQL.Calls] from accumulating statements, so a
// benchmark issuing millions of them runs in constant memory. Statements are
// still answered by the script.
func (f *FakeSQL) DisableRecording() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.noRecord = true
}

// RowsClosed returns how many result sets were closed.
func (f *FakeSQL) RowsClosed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// Commits returns how many transactions were committed.
func (f *FakeSQL) Commits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commits
}

// Rollbacks returns how many transactions were rolled back.
func (f *FakeSQL) Rollbacks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rollbacks
}

func (f *FakeSQL) record(query string, args []driver.NamedValue, inTx bool) FakeSQLReply {
	values := make([]any, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	f.mu.Lock()
	if !f.noRecord {
		f.calls = append(f.calls, FakeSQLCall{Query: query, Args: values, InTx: inTx})
	}
	respond := f.respond
	f.mu.Unlock()
	return respond(query, values)
}

// Connect implements driver.Connector.
func (f *FakeSQL) Connect(context.Context) (driver.Conn, error) { return &fakeSQLConn{db: f}, nil }

// Driver implements driver.Connector.
func (f *FakeSQL) Driver() driver.Driver { return fakeSQLDriver{f} }

type fakeSQLDriver struct{ db *FakeSQL }

func (d fakeSQLDriver) Open(string) (driver.Conn, error) { return &fakeSQLConn{db: d.db}, nil }

type fakeSQLConn struct {
	db   *FakeSQL
	inTx bool
}

func (c *fakeSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeSQLStmt{conn: c, query: query}, nil
}
func (c *fakeSQLConn) Close() error { return nil }
func (c *fakeSQLConn) Begin() (driver.Tx, error) {
	c.inTx = true
	return &fakeSQLTx{conn: c}, nil
}

// BeginTx accepts any isolation level, so code that asks for one is testable.
func (c *fakeSQLConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return c.Begin() }

func (c *fakeSQLConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	r := c.db.record(query, args, c.inTx)
	if r.Err != nil {
		return nil, r.Err
	}
	return driver.RowsAffected(r.Affected), nil
}

func (c *fakeSQLConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	r := c.db.record(query, args, c.inTx)
	if r.Err != nil {
		return nil, r.Err
	}
	return &fakeSQLRows{db: c.db, columns: r.Columns, rows: r.Rows}, nil
}

// fakeSQLStmt records each execution of a prepared statement like a direct one.
type fakeSQLStmt struct {
	conn  *fakeSQLConn
	query string
}

func (s *fakeSQLStmt) Close() error  { return nil }
func (s *fakeSQLStmt) NumInput() int { return -1 }

func (s *fakeSQLStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("testhelpers: FakeSQL needs ExecContext")
}

func (s *fakeSQLStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("testhelpers: FakeSQL needs QueryContext")
}

func (s *fakeSQLStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *fakeSQLStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

type fakeSQLTx struct{ conn *fakeSQLConn }

func (t *fakeSQLTx) Commit() error {
	t.conn.inTx = false
	t.conn.db.mu.Lock()
	t.conn.db.commits++
	t.conn.db.mu.Unlock()
	return nil
}

func (t *fakeSQLTx) Rollback() error {
	t.conn.inTx = false
	t.conn.db.mu.Lock()
	t.conn.db.rollbacks++
	t.conn.db.mu.Unlock()
	return nil
}

type fakeSQLRows struct {
	db      *FakeSQL
	columns []string
	rows    [][]driver.Value
	i       int
}

func (r *fakeSQLRows) Columns() []string { return r.columns }

func (r *fakeSQLRows) Close() error {
	r.db.mu.Lock()
	r.db.closed++
	r.db.mu.Unlock()
	return nil
}

func (r *fakeSQLRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}
