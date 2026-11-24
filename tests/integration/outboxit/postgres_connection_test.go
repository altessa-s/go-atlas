// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"crypto/tls"
	"database/sql/driver"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

func TestPostgresConnectionLoss(t *testing.T) {
	t.Parallel()
	spec := sqlSpecs()[0]
	f := newSQLFixture(t, spec)
	cfg, err := pgx.ParseConfig(spec.DSN())
	require.NoError(t, err)
	var current atomic.Pointer[pgx.Conn]
	db := stdlib.OpenDB(*cfg,
		stdlib.OptionShouldPing(func(context.Context, stdlib.ShouldPingParams) bool { return true }),
		stdlib.OptionAfterConnect(func(_ context.Context, session *pgx.Conn) error {
			current.Store(session)
			return nil
		}),
	)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := outboxsql.New(db, spec.dialect, outboxsql.WithTableName(f.sql.events))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Close only this client's socket. A transaction pooler can immediately
	// assign the server PID from a completed query to a different client.
	require.NoError(t, db.PingContext(ctx))
	before := current.Load()
	require.NoError(t, before.PgConn().Conn().Close())
	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	require.Zero(t, stats.Pending)
	require.NotSame(t, before, current.Load())

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, current.Load().PgConn().Conn().Close())
	err = store.SaveEvents(outboxsql.WithTx(ctx, tx), event("lost transaction", "body"))
	require.Error(t, err)
	require.Error(t, tx.Commit())
	require.Zero(t, f.sql.count(t, f.sql.events))
}

// dropWriteConn can sever a real PostgreSQL connection before sending any
// bytes of its next write. The driver can then classify the failure precisely.
type dropWriteConn struct {
	net.Conn
	drop atomic.Bool
}

func (c *dropWriteConn) Write(p []byte) (int, error) {
	if c.drop.Swap(false) {
		_ = c.Conn.Close()
		return 0, net.ErrClosed
	}
	return c.Conn.Write(p)
}

func TestPostgresRetryBeforeSend(t *testing.T) {
	t.Parallel()
	spec := sqlSpecs()[0]
	f := newSQLFixture(t, spec)
	cfg, err := pgx.ParseConfig(spec.DSN())
	require.NoError(t, err)
	var current atomic.Pointer[dropWriteConn]
	var connections atomic.Int32
	cfg.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	cfg.StatementCacheCapacity = 16
	dial := cfg.DialFunc
	cfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &dropWriteConn{Conn: conn}, nil
	}
	db := stdlib.OpenDB(*cfg,
		stdlib.OptionShouldPing(func(context.Context, stdlib.ShouldPingParams) bool { return false }),
		stdlib.OptionAfterConnect(func(_ context.Context, session *pgx.Conn) error {
			conn := session.PgConn().Conn()
			if encrypted, ok := conn.(*tls.Conn); ok {
				conn = encrypted.NetConn()
			}
			current.Store(conn.(*dropWriteConn))
			connections.Add(1)
			return nil
		}),
	)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := outboxsql.New(db, spec.dialect, outboxsql.WithTableName(f.sql.events))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// First verify that the same failure on plain database/sql reaches the
	// caller from pgx's first-row preload, bypassing its ErrBadConn retry.
	const query = "SELECT $1::integer"
	var value int
	require.NoError(t, db.QueryRowContext(ctx, query, 1).Scan(&value))
	current.Load().drop.Store(true)
	err = db.QueryRowContext(ctx, query, 1).Scan(&value)
	require.Error(t, err)
	require.NotErrorIs(t, err, driver.ErrBadConn)
	require.True(t, pgconn.SafeToRetry(err))

	// Warm the store's query on the replacement, then inject the same fault.
	_, err = store.Stats(ctx)
	require.NoError(t, err)
	warm := connections.Load()
	current.Load().drop.Store(true)
	_, err = store.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, warm+1, connections.Load())

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	current.Load().drop.Store(true)
	err = store.SaveEvents(outboxsql.WithTx(ctx, tx), event("failed before send", "body"))
	require.Error(t, err)
	require.Error(t, tx.Commit())
	require.Equal(t, warm+1, connections.Load())
	require.Zero(t, f.sql.count(t, f.sql.events))
}
