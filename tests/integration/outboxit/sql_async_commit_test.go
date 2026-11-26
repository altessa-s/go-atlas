// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

func TestPostgresAsynchronousDispatchCommit(t *testing.T) {
	t.Parallel()
	spec := sqlSpecs()[0]
	f := newSQLFixture(t, spec)
	b, ctx := f.sql, t.Context()
	b.db.SetMaxOpenConns(1)
	b.exec(t, "SET synchronous_commit = on")
	store, err := outboxsql.New(b.db, b.dialect, outboxsql.WithTableName(b.events), outboxsql.WithAsynchronousCommit())
	require.NoError(t, err)
	// Check the setting inside each transaction at the point its write executes.
	b.exec(t, "ALTER TABLE "+b.events+" ADD CONSTRAINT save_durability CHECK (current_setting('synchronous_commit') = 'on')")
	events := []outbox.Event{
		{Id: "send", Key: "k", Status: outbox.StatusPending},
		{Id: "expire", Key: "k", Status: outbox.StatusPending, ExpiresAt: time.Now().Add(-time.Hour)},
	}
	tx, err := b.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, store.SaveEvents(outboxsql.WithTx(ctx, tx), events...))
	require.NoError(t, tx.Commit())
	b.exec(t, "ALTER TABLE "+b.events+" DROP CONSTRAINT save_durability, "+
		"ADD CONSTRAINT dispatch_durability CHECK (current_setting('synchronous_commit') = 'off') NOT VALID")
	claim := func() outbox.Event {
		t.Helper()
		claimed, err := store.FetchUnprocessedEvents(ctx, 10)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		return claimed[0]
	}
	first := claim()
	require.NoError(t, store.UnlockStuckEvents(ctx, 0))
	sent := claim()
	require.NotEqual(t, first.LockToken, sent.LockToken)
	sent.Status = outbox.StatusSent
	require.NoError(t, store.UpdateEvents(ctx, sent))
	require.Equal(t, string(outbox.StatusSent), f.load(t, sent.Id).Status)
	n, err := store.ExpireEvents(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	var restored string
	require.NoError(t, b.db.QueryRowContext(ctx, "SHOW synchronous_commit").Scan(&restored))
	require.Equal(t, "on", restored)
}

func TestSQLDispatchIgnoresBusinessTransaction(t *testing.T) {
	t.Parallel()
	forEachSQL(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		resolve := func(ctx context.Context) (*sql.Tx, bool, error) {
			tx, ok := ctx.Value(sqlTxKey{}).(*sql.Tx)
			return tx, ok, nil
		}
		store, err := outboxsql.New(f.sql.db, f.sql.dialect, outboxsql.WithTableName(f.sql.events),
			outboxsql.WithAsynchronousCommit(), outboxsql.WithTxResolver(resolve))
		require.NoError(t, err)
		f.store = store
		ob := f.newOutbox(t)
		tx, err := f.sql.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		txCtx := context.WithValue(ctx, sqlTxKey{}, tx)
		require.NoError(t, ob.Save(txCtx, event("orders", "dispatch")))
		require.NoError(t, ob.RunDispatchCycle(txCtx))
		require.Zero(t, f.recorder.Count())
		require.NoError(t, tx.Rollback())
		require.NoError(t, ob.RunDispatchCycle(ctx))
		require.Zero(t, f.recorder.Count())
		require.Zero(t, f.countEvents(t))
	})
}
