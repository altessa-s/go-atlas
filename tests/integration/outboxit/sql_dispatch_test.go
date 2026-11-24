// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// Separate connections compete for the same queue. Keep each wave of claims
// open until every dispatcher has returned, then settle only the expected heads.
func TestSQLOrderKeyConcurrentClaims(t *testing.T) {
	t.Parallel()
	forEachSQL(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		store, err := outboxsql.New(f.sql.db, f.sql.dialect, outboxsql.WithTableName(f.sql.events), outboxsql.WithKeyOrder())
		require.NoError(t, err)
		require.NoError(t, store.EnsureSchema(ctx))
		prefix := strings.Repeat("k", 8192)
		groupA, groupB := prefix+"a", prefix+"b"
		events := []outbox.Event{
			{Id: "a1", Key: "created", OrderKey: groupA},
			{Id: "a2", Key: "updated", OrderKey: groupA},
			{Id: "a3", Key: "deleted", OrderKey: groupA},
			{Id: "b1", Key: "created", OrderKey: groupB},
			{Id: "b2", Key: "updated", OrderKey: groupB},
			{Id: "c1", Key: "other"},
			{Id: "c2", Key: "other"},
		}
		for i := range events {
			events[i].Status = outbox.StatusPending
			events[i].CreatedAt = time.Now().Add(-time.Duration(i) * time.Minute)
		}
		require.NoError(t, store.SaveEvents(ctx, events...))
		var nullKeys int
		require.NoError(t, f.sql.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+f.sql.events+" WHERE order_key IS NULL").Scan(&nullKeys))
		require.Zero(t, nullKeys)
		claim := func(expected ...string) map[string]outbox.Event {
			t.Helper()
			const workers = 4
			var batches [workers][]outbox.Event
			var errs [workers]error
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range workers {
				wg.Go(func() {
					<-start
					batches[i], errs[i] = store.FetchUnprocessedEvents(ctx, 10)
				})
			}
			close(start)
			wg.Wait()
			found := map[string]outbox.Event{}
			var ids []string
			for i, batch := range batches {
				require.NoError(t, errs[i])
				for _, ev := range batch {
					ids = append(ids, ev.Id)
					found[ev.Id] = ev
				}
			}
			require.ElementsMatch(t, expected, ids)
			return found
		}
		first := claim("a1", "b1", "c1")
		require.Equal(t, "other", first["c1"].OrderKey)
		require.Equal(t, groupA, first["a1"].OrderKey)
		claim()
		a, b, c := first["a1"], first["b1"], first["c1"]
		a.Status, a.RetryAfter = outbox.StatusFailed, time.Hour
		b.Status, c.Status = outbox.StatusSent, outbox.StatusSent
		require.NoError(t, store.UpdateEvents(ctx, a, b, c))
		claim("b2", "c2")
		f.sql.exec(t, "UPDATE "+f.sql.events+" SET next_attempt_at = NULL WHERE id = ?", a.Id)
		a = claim("a1")["a1"]
		a.Status = outbox.StatusRejected
		require.NoError(t, store.UpdateEvents(ctx, a))
		second := claim("a2")["a2"]
		second.Status = outbox.StatusSent
		require.NoError(t, store.UpdateEvents(ctx, second))
		claim("a3")
	})
}

func TestSQLOrderKeyReverseCommit(t *testing.T) {
	t.Parallel()
	forEachSQL(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		store, err := outboxsql.New(f.sql.db, f.sql.dialect, outboxsql.WithTableName(f.sql.events), outboxsql.WithKeyOrder())
		require.NoError(t, err)
		low := outbox.Event{Id: "low", Key: "created", OrderKey: "order", Status: outbox.StatusPending}
		high := outbox.Event{Id: "high", Key: "updated", OrderKey: "order", Status: outbox.StatusPending}
		tx, err := f.sql.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		require.NoError(t, store.SaveEvents(outboxsql.WithTx(ctx, tx), low))
		require.NoError(t, store.SaveEvents(ctx, high))
		claimed, err := store.FetchUnprocessedEvents(ctx, 2)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		require.Equal(t, high.Id, claimed[0].Id)
		require.NoError(t, tx.Commit())
		blocked, err := store.FetchUnprocessedEvents(ctx, 2)
		require.NoError(t, err)
		require.Empty(t, blocked)
		claimed[0].Status = outbox.StatusSent
		require.NoError(t, store.UpdateEvents(ctx, claimed...))
		claimed, err = store.FetchUnprocessedEvents(ctx, 2)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		require.Equal(t, low.Id, claimed[0].Id)
	})
}

// Pause a competing claim after it locks the higher row and before it commits
// its status change. A plain snapshot would still see that row as pending.
func TestSQLOrderKeyUncommittedClaim(t *testing.T) {
	t.Parallel()
	forEachSQL(t, func(t *testing.T, f *fixture) {
		b, ctx := f.sql, t.Context()
		store, err := outboxsql.New(b.db, b.dialect, outboxsql.WithTableName(b.events), outboxsql.WithKeyOrder())
		require.NoError(t, err)
		producer, err := b.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = producer.Rollback() }()
		low := outbox.Event{Id: "low", Key: "order", Status: outbox.StatusPending}
		high := outbox.Event{Id: "high", Key: "order", Status: outbox.StatusPending}
		require.NoError(t, store.SaveEvents(outboxsql.WithTx(ctx, producer), low))
		require.NoError(t, store.SaveEvents(ctx, high))
		claim, err := b.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = claim.Rollback() }()
		_, err = claim.ExecContext(ctx, b.bind("UPDATE "+b.events+" SET status = 'in-progress' WHERE id = ?"), high.Id)
		require.NoError(t, err)
		require.NoError(t, producer.Commit())
		events, err := store.FetchUnprocessedEvents(ctx, 2)
		require.NoError(t, err)
		require.Empty(t, events)
		require.NoError(t, claim.Rollback())
		events, err = store.FetchUnprocessedEvents(ctx, 2)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, low.Id, events[0].Id)
	})
}

func TestSQLOrderKeySchemaUpgrade(t *testing.T) {
	t.Parallel()
	forEachSQL(t, func(t *testing.T, f *fixture) {
		b := f.sql
		b.exec(t, "ALTER TABLE "+b.events+" DROP COLUMN order_key")
		b.exec(t, "INSERT INTO "+b.events+" (id, topic, status, created_at) VALUES (?, ?, 'pending', CURRENT_TIMESTAMP)", "old", []byte("legacy"))
		// Migrate the existing table before starting a store that reads order_key.
		if b.dialect == outboxsql.DialectPostgres {
			b.exec(t, "ALTER TABLE "+b.events+" ADD COLUMN order_key BYTEA")
		} else {
			b.exec(t, "ALTER TABLE "+b.events+" ADD COLUMN order_key LONGBLOB NULL")
			b.exec(t, "ALTER TABLE "+b.events+" ADD INDEX "+b.events+"_order_key_idx (order_key(255), status, seq)")
		}
		store, err := outboxsql.New(b.db, b.dialect, outboxsql.WithKeyOrder(), outboxsql.WithTableName(b.events))
		require.NoError(t, err)
		require.NoError(t, store.EnsureSchema(t.Context()))
		require.Contains(t, b.catalogIndexes(t)[b.events+"_order_key_idx"], "order_key")
		var nullKeys int
		require.NoError(t, b.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+b.events+" WHERE order_key IS NULL").Scan(&nullKeys))
		require.Equal(t, 1, nullKeys)
		// A writer still running the previous version omits order_key after the upgrade.
		b.exec(t, "INSERT INTO "+b.events+" (id, topic, status, created_at) VALUES (?, ?, 'pending', CURRENT_TIMESTAMP)", "during-rollout", []byte("legacy"))
		require.NoError(t, store.SaveEvents(t.Context(), outbox.Event{Id: "new", Key: "another-topic", OrderKey: "legacy", Status: outbox.StatusPending}))
		for _, id := range []string{"old", "during-rollout", "new"} {
			claimed, err := store.FetchUnprocessedEvents(t.Context(), 10)
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, id, claimed[0].Id)
			require.Equal(t, "legacy", claimed[0].OrderKey)
			claimed[0].Status = outbox.StatusSent
			require.NoError(t, store.UpdateEvents(t.Context(), claimed...))
		}
	})
}
