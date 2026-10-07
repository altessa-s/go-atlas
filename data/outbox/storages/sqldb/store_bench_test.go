// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/data/outbox/storages/sqldb"
)

// benchConn is a database/sql connector for benchmarks. Unlike
// testhelpers.FakeSQL it records nothing, so live memory stays flat however
// many iterations b.Loop runs: every SELECT returns rows, every other
// statement affects one row, and transactions are no-ops.
type benchConn struct{ rows [][]driver.Value }

func (c benchConn) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c benchConn) Driver() driver.Driver                        { return nil }
func (c benchConn) Prepare(query string) (driver.Stmt, error)    { return benchStmt{c, query}, nil }
func (benchConn) Close() error                                   { return nil }
func (benchConn) Begin() (driver.Tx, error)                      { return benchTx{}, nil }

func (benchConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return benchTx{}, nil }

func (benchConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (c benchConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.HasPrefix(query, "SELECT") {
		return &benchRows{}, nil
	}
	return &benchRows{rows: c.rows}, nil
}

type benchTx struct{}

func (benchTx) Commit() error   { return nil }
func (benchTx) Rollback() error { return nil }

type benchStmt struct {
	conn  benchConn
	query string
}

func (benchStmt) Close() error  { return nil }
func (benchStmt) NumInput() int { return -1 }

func (s benchStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(1), nil }
func (s benchStmt) Query([]driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, nil)
}

type benchRows struct {
	rows [][]driver.Value
	i    int
}

func (*benchRows) Columns() []string {
	return []string{"id", "topic", "payload", "attempts", "error", "c", "p", "l", "e"}
}
func (*benchRows) Close() error { return nil }
func (r *benchRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

func benchStore(b *testing.B, d sqldb.Dialect, rows [][]driver.Value) *sqldb.Store {
	b.Helper()
	db := sql.OpenDB(benchConn{rows: rows})
	b.Cleanup(func() { _ = db.Close() })
	store, err := sqldb.New(db, d)
	if err != nil {
		b.Fatal(err)
	}
	return store
}

// BenchmarkSaveEvents measures building the multi-row INSERT for a batch; the
// benchmark connector makes the round trip itself negligible.
func BenchmarkSaveEvents(b *testing.B) {
	store := benchStore(b, sqldb.DialectMySQL, nil)
	events := make([]outbox.Event, 100)
	for i := range events {
		events[i] = outbox.Event{Id: strconv.Itoa(i), Key: "orders.created", Payload: []byte(`{"n":1}`),
			Status: outbox.StatusPending, CreatedAt: time.Now()}
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := store.SaveEvents(b.Context(), events...); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFetchUnprocessedEvents measures one dispatch cycle's fetch of a
// 100-event batch per dialect: scanning the rows, including the timestamp
// columns, and building the lock statement.
func BenchmarkFetchUnprocessedEvents(b *testing.B) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		dialect sqldb.Dialect
		ts      driver.Value
	}{
		{sqldb.DialectPostgres, created},
		{sqldb.DialectMySQL, created.Format("2006-01-02 15:04:05.000000")},
	} {
		rows := make([][]driver.Value, 100)
		for i := range rows {
			rows[i] = []driver.Value{[]byte(strconv.Itoa(i)), []byte("orders.created"), []byte(`{"n":1}`), int64(0), nil, tc.ts, nil, nil, tc.ts}
		}
		b.Run(string(tc.dialect), func(b *testing.B) {
			store := benchStore(b, tc.dialect, rows)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := store.FetchUnprocessedEvents(b.Context(), 100); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkUpdateEvents measures the per-event fenced writes of one dispatch
// cycle's results.
func BenchmarkUpdateEvents(b *testing.B) {
	store := benchStore(b, sqldb.DialectMySQL, nil)
	events := make([]outbox.Event, 100)
	for i := range events {
		events[i] = outbox.Event{Id: strconv.Itoa(i), Status: outbox.StatusFailed, Attempts: 1, LockToken: "t", RetryAfter: time.Second}
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := store.UpdateEvents(b.Context(), events...); err != nil {
			b.Fatal(err)
		}
	}
}
