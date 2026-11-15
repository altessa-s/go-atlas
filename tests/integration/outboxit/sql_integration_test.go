// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// Payload nil-ness survives the store on every backend: nil stays nil, an empty
// payload stays empty, and bytes come back unchanged.
func TestStore_PayloadsRoundTrip(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		ob := f.newOutbox(t)
		saved := f.save(t, ob,
			outbox.Event{Key: "k.nil"},
			outbox.Event{Key: "k.empty", Payload: []byte{}},
			outbox.Event{Key: "k.bytes", Payload: []byte{0, 1, 0xff}},
		)

		events, err := f.store.FetchUnprocessedEvents(t.Context(), 10)
		require.NoError(t, err)
		got := map[string][]byte{}
		for _, ev := range events {
			got[ev.Id] = ev.Payload
		}
		require.Nil(t, got[saved[0].Id])
		require.Empty(t, got[saved[1].Id])
		require.Equal(t, []byte{0, 1, 0xff}, got[saved[2].Id])
	})
}

// Unbounded values fit: a key and a handler error larger than 64 KB (a MySQL
// BLOB/TEXT limit) are stored, and a huge error on one event does not roll back
// the result of a successful event updated in the same batch.
func TestStore_LargeKeysAndErrors(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		bigKey := "k." + strings.Repeat("k", 70*1024)
		bigErr := errors.New(strings.Repeat("e", 70*1024))
		f.recorder.Respond(func(d outboxit.Delivery) error {
			if d.Key == bigKey {
				return bigErr
			}
			return nil
		})
		ob := f.newOutbox(t)
		saved := f.save(t, ob, event(bigKey, "fails"), event("k.ok", "succeeds"))

		require.NoError(t, ob.RunDispatchCycle(t.Context()))

		failed := f.load(t, saved[0].Id)
		require.Equal(t, string(outbox.StatusFailed), failed.Status)
		require.Equal(t, uint32(1), failed.Attempts)
		require.Equal(t, bigKey, failed.Topic)
		require.NotNil(t, failed.LastError)
		require.Equal(t, bigErr.Error(), *failed.LastError)
		require.Equal(t, string(outbox.StatusSent), f.load(t, saved[1].Id).Status)
	})
}

// A handler error is an arbitrary string — here one containing NUL, which
// PostgreSQL TEXT rejects. It must persist like any other, without rolling back
// the result of a sibling event updated in the same batch, and the failing
// event must still spend its attempts toward the retry limit.
func TestStore_ErrorsWithNULPersist(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		nulErr := errors.New("bad\x00payload")
		f.recorder.Respond(func(d outboxit.Delivery) error {
			if d.Key == "k.nul" {
				return nulErr
			}
			return nil
		})
		ob := f.newOutbox(t, outbox.WithRetryMaxAttempts(1))
		saved := f.save(t, ob, event("k.nul", "fails"), event("k.ok", "succeeds"))

		require.NoError(t, ob.RunDispatchCycle(t.Context()))

		failed := f.load(t, saved[0].Id)
		require.Equal(t, string(outbox.StatusMaxAttemptReached), failed.Status)
		require.Equal(t, uint32(1), failed.Attempts)
		require.NotNil(t, failed.LastError)
		require.Equal(t, nulErr.Error(), *failed.LastError)
		require.Equal(t, string(outbox.StatusSent), f.load(t, saved[1].Id).Status)
	})
}

// Client-supplied instants survive the round trip exactly (to the millisecond,
// MongoDB's BSON date precision) regardless of the driver's time zone settings
// — the MySQL backend runs with loc=Asia/Tokyo — and the backlog age is
// measured on the database clock.
func TestStore_InstantsAreNotShifted(t *testing.T) {
	t.Parallel()

	forEachBackend(t, func(t *testing.T, f *fixture) {
		created := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
		ob := f.newOutbox(t)
		saved := f.save(t, ob, outbox.Event{Key: "k.time", Payload: []byte("x"), CreatedAt: created, ExpiresAt: created.Add(time.Hour)})

		require.Equal(t, created, f.load(t, saved[0].Id).CreatedAt)

		events, err := f.store.FetchUnprocessedEvents(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, events, 1, "an event expiring in an hour must still be due")
		require.Equal(t, created, events[0].CreatedAt)
		require.Equal(t, created.Add(time.Hour), events[0].ExpiresAt)
		require.NoError(t, f.store.UnlockStuckEvents(t.Context(), 0))

		stats, err := f.store.Stats(t.Context())
		require.NoError(t, err)
		require.InDelta(t, time.Minute.Seconds(), stats.OldestPendingAge.Seconds(), 30, "age is a minute, not hours off")
	})
}

// database/sql has no notification API, so a SQL store reports Watch as
// unsupported and dispatch stays on the poll schedule.
func TestStore_SQLWatchIsUnsupported(t *testing.T) {
	t.Parallel()

	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			f := newSQLFixture(t, spec)
			require.ErrorIs(t, f.newOutbox(t).Watch(t.Context()), outbox.ErrWatchUnsupported)
		})
	}
}

// forEachSQL runs a scenario that only concerns the SQL store against every
// SQL backend, each in its own parallel subtest over its own tables.
func forEachSQL(t *testing.T, scenario func(t *testing.T, f *fixture)) {
	t.Helper()
	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			scenario(t, newSQLFixture(t, spec))
		})
	}
}

// exec runs a statement written with ? placeholders against the events table.
func (b *sqlBackend) exec(tb testing.TB, query string, args ...any) {
	tb.Helper()
	_, err := b.db.ExecContext(tb.Context(), b.bind(query), args...)
	require.NoError(tb, err)
}

// hoursAgo renders the database clock moved back by h hours.
func (b *sqlBackend) hoursAgo(h int) string {
	if b.dialect == outboxsql.DialectPostgres {
		return fmt.Sprintf("(now() - interval '%d hours')", h)
	}
	return fmt.Sprintf("(UTC_TIMESTAMP(6) - INTERVAL %d HOUR)", h)
}

// numbered builds n pending events whose IDs share a prefix.
func numbered(prefix string, n int) []outbox.Event {
	events := make([]outbox.Event, n)
	for i := range events {
		events[i] = outbox.Event{Id: fmt.Sprintf("%s-%05d", prefix, i), Key: "k", Status: outbox.StatusPending}
	}
	return events
}

// Retention deletes in chunks of 1000, so a backlog several chunks deep is
// swept completely by one call — and only what is due: fresh successes and
// dead letters, old or not, stay.
func TestStore_SQLCleanupSweepsABacklogLargerThanOneChunk(t *testing.T) {
	t.Parallel()

	forEachSQL(t, func(t *testing.T, f *fixture) {
		const old, fresh, dead = 2500, 300, 40

		require.NoError(t, f.store.SaveEvents(t.Context(), numbered("old", old)...))
		require.NoError(t, f.store.SaveEvents(t.Context(), numbered("fresh", fresh)...))
		require.NoError(t, f.store.SaveEvents(t.Context(), numbered("dead", dead)...))
		events := f.sql.events
		f.sql.exec(t, "UPDATE "+events+" SET status = ?, published_at = "+f.sql.hoursAgo(2)+" WHERE id LIKE 'old-%'",
			string(outbox.StatusSent))
		f.sql.exec(t, "UPDATE "+events+" SET status = ?, published_at = "+f.sql.hoursAgo(0)+" WHERE id LIKE 'fresh-%'",
			string(outbox.StatusSkipped))
		f.sql.exec(t, "UPDATE "+events+" SET status = ?, published_at = "+f.sql.hoursAgo(2)+" WHERE id LIKE 'dead-%'",
			string(outbox.StatusMaxAttemptReached))

		require.NoError(t, f.store.DeleteProcessedEvents(t.Context(), time.Hour))

		require.Equal(t, int64(fresh+dead), f.countEvents(t), "every old success is gone after one sweep, nothing else")
		var oldLeft int64
		require.NoError(t, f.sql.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+events+" WHERE id LIKE 'old-%'").Scan(&oldLeft))
		require.Zero(t, oldLeft)

		// A second sweep finds nothing and is not an error.
		require.NoError(t, f.store.DeleteProcessedEvents(t.Context(), time.Hour))
		require.Equal(t, int64(fresh+dead), f.countEvents(t))
	})
}

// An event the caller did not date is stamped by the database: created "now"
// on its clock and expiring exactly the default TTL later, both measured there
// and not against this process's clock.
func TestStore_SQLUnsetInstantsComeFromTheDatabaseClock(t *testing.T) {
	t.Parallel()

	forEachSQL(t, func(t *testing.T, f *fixture) {
		const ttl = time.Hour
		ob := f.newOutbox(t, outbox.WithDefaultEventTTL(ttl))

		// ttlSeconds is expires_at - created_at, ageSeconds the database's now - created_at.
		measure := "SELECT EXTRACT(EPOCH FROM (expires_at - created_at)), EXTRACT(EPOCH FROM (now() - created_at)) FROM "
		if f.sql.dialect != outboxsql.DialectPostgres {
			measure = "SELECT TIMESTAMPDIFF(MICROSECOND, created_at, expires_at) / 1000000," +
				" TIMESTAMPDIFF(MICROSECOND, created_at, UTC_TIMESTAMP(6)) / 1000000 FROM "
		}
		measure = f.sql.bind(measure + f.sql.events + " WHERE id = ?")

		var saved []outbox.Event
		var ttlSeconds, ageSeconds float64
		err := f.sql.inTx(t, func(txCtx context.Context) error {
			saved = []outbox.Event{event("k.undated", "x")}
			if err := ob.Save(txCtx, saved...); err != nil {
				return err
			}
			if f.sql.dialect != outboxsql.DialectPostgres {
				return nil
			}
			// PostgreSQL's now() is the transaction's start time, so inside the
			// saving transaction the stamp equals it exactly — which no
			// client-supplied instant could.
			tx, _ := txCtx.Value(sqlTxKey{}).(*sql.Tx)
			return tx.QueryRowContext(txCtx, measure, saved[0].Id).Scan(&ttlSeconds, &ageSeconds)
		})
		require.NoError(t, err)
		if f.sql.dialect == outboxsql.DialectPostgres {
			require.Zero(t, ageSeconds, "created_at is the saving transaction's now()")
		}

		require.NoError(t, f.sql.db.QueryRowContext(t.Context(), measure, saved[0].Id).Scan(&ttlSeconds, &ageSeconds))
		require.InDelta(t, ttl.Seconds(), ttlSeconds, 0.000001, "expires_at is created_at plus the TTL, computed by the database")
		require.GreaterOrEqual(t, ageSeconds, 0.0, "created_at is not ahead of the database clock")
		require.Less(t, ageSeconds, 60.0)
		require.Zero(t, saved[0].CreatedAt, "the outbox left the creation time to the store")

		// An explicit creation time is kept, and the default expiry counts from it.
		created := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Millisecond)
		dated := f.save(t, ob, outbox.Event{Key: "k.dated", Payload: []byte("x"), CreatedAt: created})
		require.Equal(t, created, f.load(t, dated[0].Id).CreatedAt)
		require.NoError(t, f.sql.db.QueryRowContext(t.Context(), measure, dated[0].Id).Scan(&ttlSeconds, &ageSeconds))
		require.InDelta(t, ttl.Seconds(), ttlSeconds, 0.000001)

		// Both are still due: neither stamp put the expiry in the past.
		events, err := f.store.FetchUnprocessedEvents(t.Context(), 10)
		require.NoError(t, err)
		require.Len(t, events, 2)
		for _, ev := range events {
			require.False(t, ev.CreatedAt.IsZero())
			require.Equal(t, ev.CreatedAt.Add(ttl), ev.ExpiresAt)
		}
	})
}

// catalogIndexes reads the events table's secondary indexes from the server
// catalog: index name to its column list.
func (b *sqlBackend) catalogIndexes(tb testing.TB) map[string]string {
	tb.Helper()

	query := "SELECT index_name, GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ') FROM information_schema.statistics" +
		" WHERE table_schema = DATABASE() AND table_name = ? GROUP BY index_name"
	if b.dialect == outboxsql.DialectPostgres {
		// indexdef ends in the column list: "... USING btree (status, expires_at)".
		query = "SELECT indexname, substring(indexdef FROM '\\((.*)\\)$') FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1"
	}
	rows, err := b.db.QueryContext(tb.Context(), query, b.events)
	require.NoError(tb, err)
	defer func() { _ = rows.Close() }()

	indexes := map[string]string{}
	for rows.Next() {
		var name, columns string
		require.NoError(tb, rows.Scan(&name, &columns))
		indexes[name] = columns
	}
	require.NoError(tb, rows.Err())
	return indexes
}

// EnsureSchema leaves the server with an index for every predicate the store
// runs — the claim in its ORDER BY order, the unlock, expiry and retention
// sweeps — and gives a table created before an index existed that index too.
func TestStore_SQLIndexesExistAndAreAddedToExistingTables(t *testing.T) {
	t.Parallel()

	forEachSQL(t, func(t *testing.T, f *fixture) {
		want := map[string]string{
			f.sql.events + "_fetch_idx":     "status, created_at, seq",
			f.sql.events + "_locked_idx":    "status, locked_on",
			f.sql.events + "_published_idx": "status, published_at",
			f.sql.events + "_expires_idx":   "status, expires_at",
		}
		requireIndexes := func() {
			t.Helper()
			got := f.sql.catalogIndexes(t)
			for name, columns := range want {
				require.Equal(t, columns, got[name], "index %s; catalog has %v", name, got)
			}
		}
		requireIndexes()

		// A table from before the expiry index existed.
		expires := f.sql.events + "_expires_idx"
		if f.sql.dialect == outboxsql.DialectPostgres {
			f.sql.exec(t, "DROP INDEX "+expires)
		} else {
			f.sql.exec(t, "ALTER TABLE "+f.sql.events+" DROP INDEX "+expires)
		}
		require.NotContains(t, f.sql.catalogIndexes(t), expires)

		// Several instances upgrade it at once.
		store, ok := f.store.(*outboxsql.Store)
		require.True(t, ok)
		var wg sync.WaitGroup
		errs := make([]error, 6)
		for i := range errs {
			wg.Go(func() { errs[i] = store.EnsureSchema(t.Context()) })
		}
		wg.Wait()
		for _, err := range errs {
			require.NoError(t, err)
		}
		requireIndexes()
	})
}

// The claim takes the oldest ready events whatever their status: pending and
// due failed events are read separately, each in index order, and merged by
// creation time and insertion order.
func TestStore_SQLClaimMergesPendingAndFailedOldestFirst(t *testing.T) {
	t.Parallel()

	forEachSQL(t, func(t *testing.T, f *fixture) {
		first := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
		second := first.Add(time.Second)
		require.NoError(t, f.store.SaveEvents(t.Context(),
			outbox.Event{Id: "e1", Key: "k", Status: outbox.StatusFailed, CreatedAt: first},
			outbox.Event{Id: "e2", Key: "k", Status: outbox.StatusPending, CreatedAt: first},
			outbox.Event{Id: "e3", Key: "k", Status: outbox.StatusPending, CreatedAt: second},
			outbox.Event{Id: "e4", Key: "k", Status: outbox.StatusFailed, CreatedAt: second},
			outbox.Event{Id: "e5", Key: "k", Status: outbox.StatusFailed, CreatedAt: first},
		))
		// e5 is still backing off.
		f.sql.exec(t, "UPDATE "+f.sql.events+" SET next_attempt_at = "+f.sql.hoursAgo(-1)+" WHERE id = ?", "e5")

		events, err := f.store.FetchUnprocessedEvents(t.Context(), 3)
		require.NoError(t, err)
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.Id
		}
		require.Equal(t, []string{"e1", "e2", "e3"}, ids)

		require.Equal(t, string(outbox.StatusFailed), f.load(t, "e4").Status, "an event beyond the batch is left as it was")
		require.Empty(t, f.load(t, "e4").LockToken)

		events, err = f.store.FetchUnprocessedEvents(t.Context(), 3)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, "e4", events[0].Id)
	})
}

// UpdateEvents takes its row locks in primary-key order and, when the database
// aborts it to break a deadlock anyway, runs again: here another transaction
// holds the second row and then asks for the first.
func TestStore_SQLUpdateIsRetriedAfterADeadlock(t *testing.T) {
	t.Parallel()

	forEachSQL(t, func(t *testing.T, f *fixture) {
		require.NoError(t, f.store.SaveEvents(t.Context(), numbered("ev", 2)...))
		claimed, err := f.store.FetchUnprocessedEvents(t.Context(), 2)
		require.NoError(t, err)
		require.Len(t, claimed, 2)
		for i := range claimed {
			claimed[i].Status, claimed[i].PublishedAt, claimed[i].Attempts = outbox.StatusSent, time.Now(), 1
		}
		// Rows that make the rival transaction the more expensive one to roll
		// back, so InnoDB picks the store's transaction as the victim.
		require.NoError(t, f.store.SaveEvents(t.Context(), numbered("pad", 50)...))

		rival, err := f.sql.db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = rival.Rollback() }()
		touch := "UPDATE " + f.sql.events + " SET attempts = attempts + 1 WHERE id "
		_, err = rival.ExecContext(t.Context(), touch+"LIKE 'pad-%'")
		require.NoError(t, err)
		_, err = rival.ExecContext(t.Context(), f.sql.bind(touch+"= ?"), claimed[1].Id)
		require.NoError(t, err)

		// The store locks the first row and waits for the second.
		updated := make(chan error, 1)
		go func() { updated <- f.store.UpdateEvents(t.Context(), claimed...) }()
		time.Sleep(500 * time.Millisecond)

		// The rival closes the cycle. It survives; the store's transaction is the victim.
		_, err = rival.ExecContext(t.Context(), f.sql.bind(touch+"= ?"), claimed[0].Id)
		require.NoError(t, err, "the rival transaction must not be the deadlock victim")
		require.NoError(t, rival.Commit())

		select {
		case err := <-updated:
			require.NoError(t, err, "a deadlock must be retried, not reported")
		case <-time.After(settleWindow):
			t.Fatal("UpdateEvents did not finish")
		}
		for _, ev := range claimed {
			require.Equal(t, string(outbox.StatusSent), f.load(t, ev.Id).Status)
		}
	})
}

// Instances starting together can each create the same absent events table,
// whether they name it schema-qualified or not: EnsureSchema serializes the
// initial DDL rather than racing on the catalog.
func TestStore_ConcurrentEnsureSchema(t *testing.T) {
	t.Parallel()

	for _, spec := range sqlSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			for range 5 {
				f := newSQLFixtureWithoutSchema(t, spec)
				query := "SELECT DATABASE()"
				if spec.dialect == outboxsql.DialectPostgres {
					query = "SELECT current_schema()"
				}
				var schema string
				require.NoError(t, f.db.QueryRowContext(t.Context(), query).Scan(&schema))
				var wg sync.WaitGroup
				errs := make([]error, 8)
				for i := range errs {
					wg.Go(func() {
						// Half the instances name the table schema-qualified.
						table := f.events
						if i%2 == 1 {
							table = schema + "." + table
						}
						store, err := outboxsql.New(f.db, spec.dialect, outboxsql.WithTableName(table))
						if err == nil {
							err = store.EnsureSchema(t.Context())
						}
						errs[i] = err
					})
				}
				wg.Wait()
				for _, err := range errs {
					require.NoError(t, err)
				}
			}
		})
	}
}
