// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/saga"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	_ "github.com/jackc/pgx/v5/stdlib"

	auditsql "github.com/altessa-s/go-atlas/audit/storages/sqldb"
	locksql "github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
	idemsql "github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	retrysql "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
	sagasql "github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
)

func TestStoreRollback(t *testing.T) {
	target := livebackend.SQLTargets()[0]
	db := target.Open(t)
	schema := "bindings_" + strings.ToLower(rand.Text()[:12])
	_, err := db.ExecContext(t.Context(), "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})
	type transactionKey struct{}
	resolve := func(ctx context.Context) (*sql.Tx, bool, error) {
		tx, ok := ctx.Value(transactionKey{}).(*sql.Tx)
		return tx, ok, nil
	}
	type querier interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}
	count := func(t *testing.T, ctx context.Context, db querier, table string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		return n
	}
	ctx := t.Context()
	a, err := auditsql.New(db, auditsql.DialectPostgres, auditsql.WithTxResolver(resolve), auditsql.WithTableName(schema+".audit"), auditsql.WithMaxBatchRows(1))
	require.NoError(t, err)
	require.NoError(t, a.EnsureSchema(ctx))
	s, err := sagasql.New(db, sagasql.DialectPostgres, sagasql.WithTxResolver(resolve), sagasql.WithTableName(schema+".sagas"))
	require.NoError(t, err)
	require.NoError(t, s.EnsureSchema(ctx))
	locks, err := locksql.New(db, locksql.DialectPostgres, locksql.WithTableName(schema+".locks"))
	require.NoError(t, err)
	require.NoError(t, locks.EnsureSchema(ctx))
	t.Cleanup(func() { require.NoError(t, locks.Close(context.Background())) })
	idem, err := idemsql.New(db, idemsql.DialectPostgres, idemsql.WithTxResolver(resolve), idemsql.WithTableName(schema+".keys"))
	require.NoError(t, err)
	require.NoError(t, idem.EnsureSchema(ctx))
	t.Cleanup(func() { require.NoError(t, idem.Close(context.Background())) })
	retry, err := retrysql.New[string](db, retrysql.DialectPostgres, retrysql.WithTxResolver(resolve), retrysql.WithTableName(schema+".retries"))
	require.NoError(t, err)
	require.NoError(t, retry.EnsureSchema(ctx))
	t.Cleanup(func() { require.NoError(t, retry.Close(context.Background())) })
	events, err := outboxsql.New(db, outboxsql.DialectPostgres, outboxsql.WithTxResolver(resolve), outboxsql.WithTableName(schema+".events"))
	require.NoError(t, err)
	require.NoError(t, events.EnsureSchema(ctx))
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	txCtx := context.WithValue(ctx, transactionKey{}, tx)
	require.NoError(t, events.SaveEvents(txCtx, outbox.Event{Id: "one", Key: "order.created", Payload: []byte("payload"), Status: outbox.StatusPending}))
	_, err = retry.Mark(txCtx, retryqueue.Item[string]{Key: "one", Generation: 1, Payload: "retry"})
	require.NoError(t, err)
	acquired, _, _, err := idem.AttemptLock(txCtx, "once", []byte("holder"))
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, a.StoreBatch(txCtx, []*audit.Event{{ID: "one", Timestamp: time.Now()}, {ID: "two", Timestamp: time.Now()}}))
	n, err := a.Count(txCtx, &audit.Query{})
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	require.NoError(t, s.Create(txCtx, &saga.Instance{ID: "one", Definition: "workflow"}))
	_, err = s.Get(txCtx, "one")
	require.NoError(t, err)
	lock, err := locks.Lock(txCtx, "one")
	require.NoError(t, err)
	_, err = lock.GetLockInfo(txCtx)
	require.NoError(t, err)

	tables := map[string]int{"sagas": 1, "audit": 2, "keys": 1, "retries": 1, "events": 1}
	for table, want := range tables {
		require.Equal(t, want, count(t, txCtx, tx, schema+"."+table), table)
		require.Zero(t, count(t, ctx, db, schema+"."+table))
	}
	claimed, err := events.FetchUnprocessedEvents(txCtx, 1)
	require.NoError(t, err)
	require.Empty(t, claimed)
	require.NoError(t, tx.Rollback())
	claimed, err = events.FetchUnprocessedEvents(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, claimed)
	_, err = lock.GetLockInfo(ctx)
	require.NoError(t, err)
	require.NoError(t, lock.Release(ctx))
	for table := range tables {
		require.Zero(t, count(t, ctx, db, schema+"."+table))
	}
}
