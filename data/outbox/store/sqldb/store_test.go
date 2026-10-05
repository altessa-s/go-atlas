// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var errBoom = errors.New("boom")

var numberedParamRE = regexp.MustCompile(`\$\d+`)

func ok(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Affected: 1} }

func newStore(t *testing.T, d sqldb.Dialect, respond func(string, []any) testhelpers.FakeSQLReply) (*sqldb.Store, *sql.DB, *testhelpers.FakeSQL) {
	t.Helper()
	db, fake := testhelpers.NewFakeSQL(t, respond)
	store, err := sqldb.New(db, d)
	require.NoError(t, err)
	return store, db, fake
}

func pending(id string) outbox.Event {
	return outbox.Event{Id: id, Key: "orders.created", Status: outbox.StatusPending, CreatedAt: time.Now()}
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, ok)

	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New(db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	for _, name := range []string{"t; DROP TABLE x", "a.b.c", "1events", "ev ents", strings.Repeat("t", 64)} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName(name))
		require.ErrorIs(t, err, sqldb.ErrInvalidTableName, name)
	}
	_, err = sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTableName("app.events"))
	require.NoError(t, err)
}

func TestNewDoesNoIO(t *testing.T) {
	t.Parallel()
	for _, d := range []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL} {
		_, _, fake := newStore(t, d, ok)
		require.Empty(t, fake.Calls(), d)
	}
}

func TestEnsureSchema(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dialect sqldb.Dialect
		indexes int
		locked  bool // PostgreSQL: one transaction under an advisory lock
	}{
		{sqldb.DialectPostgres, 3, true},
		{sqldb.DialectMySQL, 0, false},
	} {
		t.Run(string(tc.dialect), func(t *testing.T) {
			t.Parallel()
			store, _, fake := newStore(t, tc.dialect, ok)
			require.NoError(t, store.EnsureSchema(t.Context()))
			calls := fake.Calls()
			if tc.locked {
				require.Equal(t, "SELECT pg_advisory_xact_lock($1)", calls[0].Query)
				calls = calls[1:]
				require.Equal(t, 1, fake.Commits())
			}
			require.Len(t, calls, 1+tc.indexes)
			require.Contains(t, calls[0].Query, "CREATE TABLE IF NOT EXISTS")
			require.Regexp(t, `payload\s+\w+\s+NULL`, calls[0].Query, "nil payloads are valid")
			for _, c := range calls {
				require.Equal(t, tc.locked, c.InTx, c.Query)
			}
			for _, c := range calls[1:] {
				require.Contains(t, c.Query, "CREATE INDEX IF NOT EXISTS")
			}

			n := len(fake.Calls())
			require.NoError(t, store.EnsureSchema(t.Context()), "the DDL is idempotent")
			require.Equal(t, fake.Calls()[:n], fake.Calls()[n:], "a second run issues the same statements")
		})
	}

	store, _, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: errBoom} })
	require.ErrorIs(t, store.EnsureSchema(t.Context()), errBoom)
}

func TestSaveEventsTransactions(t *testing.T) {
	t.Parallel()
	store, db, fake := newStore(t, sqldb.DialectPostgres, ok)
	ctx := t.Context()

	require.NoError(t, store.SaveEvents(ctx, pending("e1")))
	require.False(t, fake.Calls()[0].InTx, "a single INSERT needs no transaction of its own")

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, store.SaveEvents(sqldb.WithTx(ctx, tx), pending("e2")))
	require.NoError(t, tx.Commit())
	calls := fake.Calls()
	require.True(t, calls[len(calls)-1].InTx, "SaveEvents must run on the caller's transaction")

	// More rows than one INSERT holds: still all-or-nothing.
	big := make([]outbox.Event, 501)
	for i := range big {
		big[i] = pending("b" + string(rune('a'+i%26)) + strings.Repeat("x", i/26))
	}
	commits := fake.Commits()
	require.NoError(t, store.SaveEvents(ctx, big...))
	calls = fake.Calls()
	require.True(t, calls[len(calls)-1].InTx)
	require.True(t, calls[len(calls)-2].InTx)
	require.Equal(t, commits+1, fake.Commits())
}

func TestPlaceholdersAndMySQLTimeBinding(t *testing.T) {
	t.Parallel()
	for _, d := range []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			store, _, fake := newStore(t, d, ok)
			ctx := t.Context()
			ev := pending("e1")
			ev.ExpiresAt = time.Now().Add(time.Hour)
			require.NoError(t, store.SaveEvents(ctx, ev))
			require.NoError(t, store.UnlockStuckEvents(ctx, time.Minute))
			require.NoError(t, store.DeleteProcessedEvents(ctx, time.Hour))
			_, err := store.ExpireEvents(ctx)
			require.NoError(t, err)
			failed := ev
			failed.Status, failed.LockToken, failed.RetryAfter = outbox.StatusFailed, "tok", time.Second
			require.NoError(t, store.UpdateEvents(ctx, failed))

			for _, c := range fake.Calls() {
				if d == sqldb.DialectPostgres {
					require.NotContains(t, c.Query, "?", c.Query)
					require.Len(t, numberedParamRE.FindAllString(c.Query, -1), len(c.Args), c.Query)
					continue
				}
				require.Equal(t, strings.Count(c.Query, "?"), len(c.Args), c.Query)
				for _, a := range c.Args {
					_, isTime := a.(time.Time)
					require.False(t, isTime, "MySQL must never receive a time.Time the driver could shift: %s", c.Query)
				}
				require.NotContains(t, c.Query, "NOW()", "MySQL clock reads must be UTC_TIMESTAMP")
			}
		})
	}
}

func TestFetchLocksInOneTransaction(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 2, 3, 4, 5, 600000000, time.UTC)
	store, _, fake := newStore(t, sqldb.DialectMySQL, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "SELECT") {
			return testhelpers.FakeSQLReply{
				Columns: []string{"id", "topic", "payload", "attempts", "error", "c", "p", "l", "e"},
				Rows: [][]driver.Value{
					{[]byte("e1"), []byte("k1"), nil, int64(2), []byte("prev"), "2026-01-02 03:04:05.600000", nil, nil, nil},
					{[]byte("e2"), []byte("k2"), []byte{}, int64(0), nil, "2026-01-02 03:04:06.000000", nil, nil, nil},
				},
			}
		}
		return testhelpers.FakeSQLReply{Affected: 2}
	})

	events, err := store.FetchUnprocessedEvents(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "e1", events[0].Id)
	require.Equal(t, "k1", events[0].Key)
	require.Nil(t, events[0].Payload, "a NULL payload reads back as nil")
	require.NotNil(t, events[1].Payload, "an empty payload reads back as empty, not nil")
	require.Equal(t, created, events[0].CreatedAt, "DATETIME strings are parsed as UTC")
	require.Equal(t, "prev", *events[0].LastError)
	require.Nil(t, events[1].LastError)
	require.Equal(t, outbox.StatusInProgress, events[0].Status)
	require.NotEmpty(t, events[0].LockToken)
	require.Equal(t, events[0].LockToken, events[1].LockToken)

	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.Contains(t, calls[0].Query, "FOR UPDATE SKIP LOCKED")
	require.Contains(t, calls[0].Query, "ORDER BY created_at, seq")
	require.True(t, calls[0].InTx)
	require.True(t, calls[1].InTx)
	require.Contains(t, calls[1].Query, "lock_token = ?")
	require.Equal(t, events[0].LockToken, calls[1].Args[1])
	require.Equal(t, 1, fake.Commits())
}

func TestFetchEmptyBatchLocksNothing(t *testing.T) {
	t.Parallel()
	store, _, fake := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"id"}}
	})
	events, err := store.FetchUnprocessedEvents(t.Context(), 10)
	require.NoError(t, err)
	require.Empty(t, events)
	require.NotNil(t, events)
	require.Len(t, fake.Calls(), 1)
}

func TestUpdateEventsFencesOnLockToken(t *testing.T) {
	t.Parallel()
	store, _, fake := newStore(t, sqldb.DialectPostgres, ok)
	msg := "handler failed"
	sent := outbox.Event{Id: "a", Status: outbox.StatusSent, Attempts: 1, LockToken: "t1", PublishedAt: time.Now()}
	failed := outbox.Event{Id: "b", Status: outbox.StatusFailed, Attempts: 3, LockToken: "t1", LastError: &msg, RetryAfter: 1500 * time.Millisecond}
	require.NoError(t, store.UpdateEvents(t.Context(), sent, failed))

	calls := fake.Calls()
	require.Len(t, calls, 2)
	for _, c := range calls {
		require.True(t, c.InTx)
		require.Contains(t, c.Query, "lock_token = $9")
	}
	// status, attempts, error, locked_on, published?, retry?, retry secs, id, token
	require.Equal(t, []any{"sent", int64(1), nil, nil, true, false, 0.0, "a", "t1"}, calls[0].Args)
	require.Equal(t, []any{"failed", int64(3), []byte(msg), nil, false, true, 1.5, "b", "t1"}, calls[1].Args)
}

func TestStatsAndExpireMapping(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "SELECT") {
			return testhelpers.FakeSQLReply{Columns: []string{"p", "i", "d", "a"}, Rows: [][]driver.Value{{int64(3), int64(1), int64(2), int64(1500)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 4}
	})
	st, err := store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, outbox.Stats{Pending: 3, InProgress: 1, DeadLettered: 2, OldestPendingAge: 1500 * time.Millisecond}, st)

	n, err := store.ExpireEvents(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(4), n)
}

func TestWatchIsUnsupported(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, sqldb.DialectPostgres, ok)
	ob := outbox.New(store, func(context.Context, outbox.Event) error { return nil })
	require.ErrorIs(t, ob.Watch(t.Context()), outbox.ErrWatchUnsupported)
}

func TestFetchLocksLargeBatchesInChunks(t *testing.T) {
	t.Parallel()
	rows := make([][]driver.Value, 2500)
	for i := range rows {
		rows[i] = []driver.Value{"e" + strconv.Itoa(i), []byte("k"), nil, int64(0), nil, time.Now(), nil, nil, nil}
	}
	store, _, fake := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "SELECT") {
			return testhelpers.FakeSQLReply{Columns: []string{"id", "topic", "payload", "attempts", "error", "c", "p", "l", "e"}, Rows: rows}
		}
		return testhelpers.FakeSQLReply{Affected: 1000}
	})

	events, err := store.FetchUnprocessedEvents(t.Context(), 2500)
	require.NoError(t, err)
	require.Len(t, events, 2500)
	var locks []testhelpers.FakeSQLCall
	for _, c := range fake.Calls() {
		if strings.HasPrefix(c.Query, "UPDATE") {
			locks = append(locks, c)
		}
	}
	require.Len(t, locks, 3, "2500 IDs lock in three statements")
	total := 0
	for _, c := range locks {
		require.True(t, c.InTx)
		require.LessOrEqual(t, len(c.Args), 1002)
		require.Equal(t, events[0].LockToken, c.Args[1], "one token for the whole batch")
		total += len(c.Args) - 2
	}
	require.Equal(t, 2500, total)
}
