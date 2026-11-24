// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"
	"github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// pgDeadlock has the shape of a PostgreSQL driver error: the SQLSTATE behind a
// method.
type pgDeadlock struct{ state string }

func (e *pgDeadlock) Error() string    { return "ERROR: SQLSTATE " + e.state }
func (e *pgDeadlock) SQLState() string { return e.state }

// mysqlError has the shape of go-sql-driver/mysql's error: the server error
// number in a field.
type mysqlError struct{ Number uint16 }

func (e *mysqlError) Error() string { return "Error " + strconv.Itoa(int(e.Number)) }

// byDialect runs fn once per dialect with the clock and interval expressions
// the dialect renders.
func byDialect(t *testing.T, fn func(t *testing.T, d sqldb.Dialect, now, shifted string)) {
	t.Helper()
	for _, tc := range []struct {
		dialect      sqldb.Dialect
		now, shifted string
	}{
		{sqldb.DialectPostgres, "now()", "(now() + make_interval(secs => $"},
		{sqldb.DialectMySQL, "UTC_TIMESTAMP(6)", "(UTC_TIMESTAMP(6) + INTERVAL ? MICROSECOND)"},
	} {
		t.Run(string(tc.dialect), func(t *testing.T) {
			t.Parallel()
			fn(t, tc.dialect, tc.now, tc.shifted)
		})
	}
}

// An event the caller did not date is stamped by the database — the clock every
// later predicate compares against — and so is the expiry a TTL derives.
func TestSaveStampsUnsetInstantsWithTheDatabaseClock(t *testing.T) {
	t.Parallel()
	byDialect(t, func(t *testing.T, d sqldb.Dialect, now, shifted string) {
		store, _, fake := newStore(t, d, ok)
		require.NoError(t, store.SaveEventsWithTTL(t.Context(), 90*time.Second,
			outbox.Event{Id: "e1", Key: "k", Status: outbox.StatusPending}))

		call := fake.Calls()[0]
		require.Contains(t, call.Query, ", "+now+", ", "created_at comes from the database clock")
		require.Contains(t, call.Query, shifted, "expires_at is the database clock plus the TTL")
		// id, topic, order_key, payload, status, attempts, error, ttl — no instant.
		require.Len(t, call.Args, 8)
		ttl := any(90.0)
		if d == sqldb.DialectMySQL {
			ttl = (90 * time.Second).Microseconds()
		}
		require.Equal(t, ttl, call.Args[7])
	})
}

// Without a TTL an undated event never expires, and SaveEvents is that case.
func TestSaveWithoutTTLLeavesExpiryNull(t *testing.T) {
	t.Parallel()
	byDialect(t, func(t *testing.T, d sqldb.Dialect, now, _ string) {
		store, _, fake := newStore(t, d, ok)
		require.NoError(t, store.SaveEvents(t.Context(), outbox.Event{Id: "e1", Key: "k", Status: outbox.StatusPending}))

		call := fake.Calls()[0]
		require.Contains(t, call.Query, ", "+now+", ")
		require.NotContains(t, call.Query, "INTERVAL")
		require.NotContains(t, call.Query, "make_interval")
		require.Len(t, call.Args, 8)
		require.Nil(t, call.Args[7], "expires_at is NULL")
	})
}

// Instants the caller set are part of the Event contract and are stored as
// given; a TTL then counts from the caller's creation time, with no clock read.
func TestSaveKeepsExplicitInstants(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expires := created.Add(time.Hour)

	for _, tc := range []struct {
		name string
		ev   outbox.Event
		want time.Time // expires_at bound as an argument
	}{
		{"both set", outbox.Event{CreatedAt: created, ExpiresAt: expires}, expires},
		{"created set, ttl applies", outbox.Event{CreatedAt: created}, created.Add(time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _, fake := newStore(t, sqldb.DialectPostgres, ok)
			tc.ev.Id, tc.ev.Key, tc.ev.Status = "e1", "k", outbox.StatusPending
			require.NoError(t, store.SaveEventsWithTTL(t.Context(), time.Minute, tc.ev))

			call := fake.Calls()[0]
			require.NotContains(t, call.Query, "now()")
			require.Len(t, call.Args, 9)
			require.Equal(t, created, call.Args[5])
			require.Equal(t, tc.want, call.Args[8])
		})
	}

	t.Run("expiry set, creation stamped", func(t *testing.T) {
		t.Parallel()
		store, _, fake := newStore(t, sqldb.DialectPostgres, ok)
		require.NoError(t, store.SaveEventsWithTTL(t.Context(), time.Minute,
			outbox.Event{Id: "e1", Key: "k", Status: outbox.StatusPending, ExpiresAt: expires}))

		call := fake.Calls()[0]
		require.Contains(t, call.Query, ", now(), ")
		require.NotContains(t, call.Query, "make_interval", "an explicit expiry overrides the TTL")
		require.Equal(t, expires, call.Args[7])
	})
}

// Retention deletes in bounded chunks, one statement — one short transaction —
// each, until a chunk comes back short.
func TestDeleteProcessedEventsInChunks(t *testing.T) {
	t.Parallel()
	byDialect(t, func(t *testing.T, d sqldb.Dialect, _, _ string) {
		affected := []int64{1000, 1000, 400}
		var n atomic.Int64
		store, _, fake := newStore(t, d, func(string, []any) testhelpers.FakeSQLReply {
			return testhelpers.FakeSQLReply{Affected: affected[n.Add(1)-1]}
		})
		require.NoError(t, store.DeleteProcessedEvents(t.Context(), time.Hour))

		calls := fake.Calls()
		require.Len(t, calls, len(affected), "a short chunk ends the sweep")
		for _, c := range calls {
			require.False(t, c.InTx, "each chunk runs in autocommit")
			require.Contains(t, c.Query, "LIMIT 1000")
			require.Equal(t, []any{"sent", "skipped", "expired"}, c.Args[:3])
			if d == sqldb.DialectPostgres {
				require.Contains(t, c.Query, "WHERE id IN (SELECT id FROM", "PostgreSQL has no DELETE ... LIMIT")
				require.Contains(t, c.Query, "FOR UPDATE SKIP LOCKED")
			}
		}
	})
}

// Expiry marks in bounded chunks, one statement each, until a chunk comes back
// short, and sums the chunks.
func TestExpireEventsInChunks(t *testing.T) {
	t.Parallel()
	byDialect(t, func(t *testing.T, d sqldb.Dialect, _, _ string) {
		affected := []int64{1000, 1000, 7}
		var n atomic.Int64
		store, _, fake := newStore(t, d, func(string, []any) testhelpers.FakeSQLReply {
			return testhelpers.FakeSQLReply{Affected: affected[n.Add(1)-1]}
		})
		total, err := store.ExpireEvents(t.Context())
		require.NoError(t, err)
		require.EqualValues(t, 2007, total)

		calls := fake.Calls()
		require.Len(t, calls, len(affected), "a short chunk ends the sweep")
		for _, c := range calls {
			require.False(t, c.InTx, "each chunk runs in autocommit")
			require.Contains(t, c.Query, "LIMIT 1000")
			require.Equal(t, []any{"expired", "pending", "failed"}, c.Args)
			if d == sqldb.DialectPostgres {
				require.Contains(t, c.Query, "WHERE id IN (SELECT id FROM", "PostgreSQL has no UPDATE ... LIMIT")
				require.Contains(t, c.Query, "FOR UPDATE SKIP LOCKED")
			}
		}
	})
}

// A sweep whose context ends stops between chunks and reports it.
func TestDeleteProcessedEventsStopsWhenContextEnds(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	store, _, fake := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		cancel()
		return testhelpers.FakeSQLReply{Affected: 1000}
	})
	require.ErrorIs(t, store.DeleteProcessedEvents(ctx, time.Hour), context.Canceled)
	require.Len(t, fake.Calls(), 1)
}

// Model database/sql returning ErrTxDone after cancellation wins the rollback race.
func TestUpdateEventsReportsCancellationAfterRollback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store, _, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		cancel()
		return testhelpers.FakeSQLReply{Err: sql.ErrTxDone}
	})
	require.ErrorIs(t, store.UpdateEvents(ctx, pending("e1")), context.Canceled)
}

// UpdateEvents and the unlock sweep take row locks in one order — the primary
// key — so they cannot deadlock on the rows they share.
func TestUpdateEventsLocksInPrimaryKeyOrder(t *testing.T) {
	t.Parallel()
	store, _, fake := newStore(t, sqldb.DialectPostgres, ok)
	events := []outbox.Event{
		{Id: "c", Status: outbox.StatusSent, LockToken: "t"},
		{Id: "a", Status: outbox.StatusSent, LockToken: "t"},
		{Id: "b", Status: outbox.StatusSent, LockToken: "t"},
	}
	require.NoError(t, store.UpdateEvents(t.Context(), events...))

	var ids []any
	for _, c := range fake.Calls() {
		ids = append(ids, c.Args[7])
	}
	require.Equal(t, []any{"a", "b", "c"}, ids)
	require.Equal(t, "c", events[0].Id, "the caller's slice is not reordered")
}

func TestUnlockLocksInPrimaryKeyOrder(t *testing.T) {
	t.Parallel()

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		store, _, fake := newStore(t, sqldb.DialectPostgres, ok)
		require.NoError(t, store.UnlockStuckEvents(t.Context(), time.Minute))

		calls := fake.Calls()
		require.Len(t, calls, 1)
		require.Contains(t, calls[0].Query, "ORDER BY id FOR UPDATE")
		require.Equal(t, []any{"pending", "in-progress", 60.0}, calls[0].Args)
	})

	// MySQL locks in index-scan order whatever the ORDER BY says, so the sweep
	// reads the stuck IDs in key order and updates them through the primary key.
	t.Run("mysql", func(t *testing.T) {
		t.Parallel()
		page := func(from, n int) [][]driver.Value {
			rows := make([][]driver.Value, n)
			for i := range rows {
				rows[i] = []driver.Value{[]byte("e" + strconv.Itoa(10000+from+i))}
			}
			return rows
		}
		var selects atomic.Int64
		store, _, fake := newStore(t, sqldb.DialectMySQL, func(q string, _ []any) testhelpers.FakeSQLReply {
			if !strings.HasPrefix(q, "SELECT") {
				return testhelpers.FakeSQLReply{Affected: 1}
			}
			reply := testhelpers.FakeSQLReply{Columns: []string{"id"}, Rows: page(0, 1000)}
			if selects.Add(1) > 1 {
				reply.Rows = page(1000, 2)
			}
			return reply
		})
		require.NoError(t, store.UnlockStuckEvents(t.Context(), time.Minute))

		calls := fake.Calls()
		require.Len(t, calls, 4, "two pages, each read and then updated")
		require.Contains(t, calls[0].Query, "ORDER BY id LIMIT 1000")
		require.Equal(t, []byte{}, calls[0].Args[2], "the first page starts before every ID")
		require.Equal(t, []byte("e10999"), calls[2].Args[2], "the next page starts after the last ID seen")

		for i, n := range map[int]int{1: 1000, 3: 2} {
			c := calls[i]
			require.Contains(t, c.Query, "FORCE INDEX (PRIMARY)")
			require.Equal(t, strings.Count(c.Query, "?"), len(c.Args), c.Query)
			require.Len(t, c.Args, n+3)
			require.Equal(t, "pending", c.Args[0])
			require.Equal(t, []any{"in-progress", time.Minute.Microseconds()}, c.Args[n+1:],
				"the predicate is repeated: the read took no lock")
		}
	})
}

// A deadlock or serialization failure the database reports anyway is retried a
// bounded number of times; any other failure is not.
func TestDeadlocksAreRetried(t *testing.T) {
	t.Parallel()

	deadlocks := []struct {
		name    string
		dialect sqldb.Dialect
		err     error
	}{
		{"postgres deadlock", sqldb.DialectPostgres, &pgDeadlock{"40P01"}},
		{"postgres serialization failure", sqldb.DialectPostgres, &pgDeadlock{"40001"}},
		{"mysql deadlock", sqldb.DialectMySQL, &mysqlError{1213}},
	}
	ops := []struct {
		name string
		run  func(context.Context, *sqldb.Store) error
	}{
		{"claim", func(ctx context.Context, s *sqldb.Store) error {
			_, err := s.FetchUnprocessedEvents(ctx, 1)
			return err
		}},
		{"retention", func(ctx context.Context, s *sqldb.Store) error { return s.DeleteProcessedEvents(ctx, time.Hour) }},
		{"expire", func(ctx context.Context, s *sqldb.Store) error {
			_, err := s.ExpireEvents(ctx)
			return err
		}},
		{"update", func(ctx context.Context, s *sqldb.Store) error {
			return s.UpdateEvents(ctx, outbox.Event{Id: "a", Status: outbox.StatusSent, LockToken: "t"})
		}},
		{"unlock", func(ctx context.Context, s *sqldb.Store) error { return s.UnlockStuckEvents(ctx, time.Minute) }},
	}

	for _, dl := range deadlocks {
		for _, op := range ops {
			t.Run(dl.name+"/"+op.name+"/recovers", func(t *testing.T) {
				t.Parallel()
				var failures atomic.Int64
				store, _, _ := newStore(t, dl.dialect, func(string, []any) testhelpers.FakeSQLReply {
					if failures.Add(1) <= 2 {
						return testhelpers.FakeSQLReply{Err: dl.err}
					}
					return testhelpers.FakeSQLReply{Affected: 1}
				})
				require.NoError(t, op.run(t.Context(), store))
			})

			t.Run(dl.name+"/"+op.name+"/gives up", func(t *testing.T) {
				t.Parallel()
				store, _, fake := newStore(t, dl.dialect, func(string, []any) testhelpers.FakeSQLReply {
					return testhelpers.FakeSQLReply{Err: dl.err}
				})
				require.ErrorIs(t, op.run(t.Context(), store), dl.err)
				require.Len(t, fake.Calls(), 4, "the first attempt and three retries")
			})
		}
	}

	for _, tc := range []struct {
		name    string
		dialect sqldb.Dialect
		err     error
	}{
		{"plain error", sqldb.DialectPostgres, errBoom},
		{"postgres unique violation", sqldb.DialectPostgres, &pgDeadlock{"23505"}},
		{"mysql lock wait timeout", sqldb.DialectMySQL, &mysqlError{1205}},
	} {
		t.Run(tc.name+" is not retried", func(t *testing.T) {
			t.Parallel()
			store, _, fake := newStore(t, tc.dialect, func(string, []any) testhelpers.FakeSQLReply {
				return testhelpers.FakeSQLReply{Err: tc.err}
			})
			require.ErrorIs(t, store.UnlockStuckEvents(t.Context(), time.Minute), tc.err)
			require.Len(t, fake.Calls(), 1)
		})
	}
}

func TestOrderedClaimRetriesChangedRecord(t *testing.T) {
	t.Parallel()
	var pendingReads int
	db, fake := testhelpers.NewFakeSQL(t, func(query string, args []any) testhelpers.FakeSQLReply {
		switch {
		case strings.HasPrefix(query, "UPDATE"):
			require.Equal(t, []any{[]byte("fresh")}, args[2:])
			return testhelpers.FakeSQLReply{Affected: 1}
		case strings.HasPrefix(query, "SELECT candidate.id"):
			require.Equal(t, []any{[]byte("fresh")}, args)
			return testhelpers.FakeSQLReply{Columns: []string{"id"}, Rows: [][]driver.Value{{"fresh"}}}
		case args[0] == string(outbox.StatusPending):
			pendingReads++
			id := "fresh"
			if pendingReads == 1 {
				id = "stale"
			}
			return readyRows([][]driver.Value{
				{[]byte(id), []byte("topic"), []byte("key"), nil, int64(0), nil, "2026-01-02 03:04:05.000000", nil, nil, nil, int64(1)},
			}, 1)(query, args)
		default:
			require.Equal(t, string(outbox.StatusFailed), args[0])
			if pendingReads == 1 {
				return testhelpers.FakeSQLReply{Err: &mysqlError{Number: 1020}}
			}
			return readyRows(nil, 0)(query, args)
		}
	})
	store, err := sqldb.New(db, sqldb.DialectMySQL, sqldb.WithKeyOrder())
	require.NoError(t, err)
	events, err := store.FetchUnprocessedEvents(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "fresh", events[0].Id)
	require.Equal(t, 2, pendingReads)
	require.Equal(t, 1, fake.Rollbacks())
	require.Equal(t, 1, fake.Commits())
}

// The expiry sweep has its own index on both dialects.
func TestEnsureSchemaIndexesExpiry(t *testing.T) {
	t.Parallel()
	byDialect(t, func(t *testing.T, d sqldb.Dialect, _, _ string) {
		store, _, fake := newStore(t, d, ok)
		require.NoError(t, store.EnsureSchema(t.Context()))

		var ddl strings.Builder
		for _, c := range fake.Calls() {
			ddl.WriteString(c.Query + "\n")
		}
		require.Regexp(t, `events_outbox_expires_idx\W+(ON \S+ )?\(status, expires_at\)`, ddl.String())
	})
}
