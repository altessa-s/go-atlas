// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// Backend termination runs before parallel scenarios can borrow the victim's
// backend through a transaction pooler.
func TestPostgresConnectionLoss(t *testing.T) {
	spec := sqlSpecs()[0]
	f := newSQLFixture(t, spec)
	cfg, err := pgx.ParseConfig(spec.DSN())
	require.NoError(t, err)
	db := stdlib.OpenDB(*cfg, stdlib.OptionShouldPing(func(_ context.Context, p stdlib.ShouldPingParams) bool {
		return p.IdleDuration > 0
	}))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := outboxsql.New(db, spec.dialect, outboxsql.WithTableName(f.sql.events))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// A direct control session cannot be assigned the backend it is terminating.
	controlConfig, err := pgx.ParseConfig(directDSN(spec))
	require.NoError(t, err)
	control := stdlib.OpenDB(*controlConfig)
	t.Cleanup(func() { _ = control.Close() })

	killBackend := func(pid int) {
		var killed bool
		require.NoError(t, control.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&killed))
		require.True(t, killed)
		require.EventuallyWithT(t, func(check *assert.CollectT) {
			var gone bool
			require.NoError(check, control.QueryRowContext(ctx, "SELECT NOT EXISTS (SELECT FROM pg_stat_activity WHERE pid = $1)", pid).Scan(&gone))
			require.True(check, gone)
		}, 5*time.Second, 10*time.Millisecond)
	}
	var before, after int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&before))
	killBackend(before)
	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	require.Zero(t, stats.Pending)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&after))
	require.NotEqual(t, before, after)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var txPID int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&txPID))
	killBackend(txPID)
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

	// Warm the statement cache so the failure occurs in the stdlib first-row preload.
	_, err = store.Stats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, connections.Load())
	current.Load().drop.Store(true)
	_, err = store.Stats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, connections.Load())

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	current.Load().drop.Store(true)
	err = store.SaveEvents(outboxsql.WithTx(ctx, tx), event("failed before send", "body"))
	require.Error(t, err)
	require.Error(t, tx.Commit())
	require.EqualValues(t, 2, connections.Load())
	require.Zero(t, f.sql.count(t, f.sql.events))
}
