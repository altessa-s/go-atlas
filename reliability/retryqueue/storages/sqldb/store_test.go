// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"
	"github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
)

var (
	errBoom         = errors.New("boom")
	numberedParamRE = regexp.MustCompile(`\$\d+`)
	dialects        = []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL}
	base            = time.Unix(1_700_000_000, 123456789).UTC()
)

// columns is the scan order of the items table.
var columns = []string{"id", "generation", "state", "since", "attempts", "next_attempt_at", "expired_at", "resolved_at",
	"flagged", "flag_reason", "payload"}

type payload struct {
	Note string `json:"note"`
}

// pgError is the shape of a PostgreSQL driver error.
type pgError struct{ code string }

func (e pgError) Error() string    { return "pg error " + e.code }
func (e pgError) SQLState() string { return e.code }

// mysqlError is the shape of a go-sql-driver/mysql error.
type mysqlError struct{ Number uint16 }

func (e *mysqlError) Error() string { return fmt.Sprintf("Error %d", e.Number) }

// duplicate returns the duplicate-key error of d.
func duplicate(d sqldb.Dialect) error {
	if d == sqldb.DialectPostgres {
		return pgError{"23505"}
	}
	return &mysqlError{Number: 1062}
}

func newStore(t *testing.T, d sqldb.Dialect, respond func(query string, args []any) testhelpers.FakeSQLReply,
	opts ...sqldb.Option,
) (*sqldb.Store[payload], *testhelpers.FakeSQL, *sql.DB) {
	t.Helper()
	db, fake := testhelpers.NewFakeSQL(t, respond)
	store, err := sqldb.New[payload](db, d, opts...)
	require.NoError(t, err)
	return store, fake, db
}

// row is a stored item as the fake database returns it.
func row(key string, state retryqueue.State) []driver.Value {
	return []driver.Value{key, int64(3), string(state), base.UnixNano(), int64(2), base.Add(time.Minute).UnixNano(),
		int64(0), int64(0), true, []byte("why"), []byte(`{"note":"stored"}`)}
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	_, err := sqldb.New[payload](nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New[payload](db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	for _, name := range []string{"t; DROP TABLE x", "a.b.c", "1rq", `"quoted"`, "r q", strings.Repeat("t", 64)} {
		_, err = sqldb.New[payload](db, sqldb.DialectPostgres, sqldb.WithTableName(name))
		require.ErrorIs(t, err, sqldb.ErrInvalidTableName, name)
	}
	_, err = sqldb.New[payload](db, sqldb.DialectMySQL, sqldb.WithTableName("app.retry_items"))
	require.NoError(t, err)
}

// TestPlaceholderStyle pins that every statement uses the dialect's
// placeholders and quoting.
func TestPlaceholderStyle(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			store, fake, _ := newStore(t, d, func(q string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(q, "SELECT") {
					return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{row("k", retryqueue.StatePending)}}
				}
				return testhelpers.FakeSQLReply{Affected: 1}
			}, sqldb.WithResolvedRetention(time.Hour))
			ctx := t.Context()
			_, err := store.Mark(ctx, retryqueue.Item[payload]{Key: "k"})
			require.NoError(t, err)
			_, err = store.Reopen(ctx, retryqueue.Item[payload]{Key: "k"})
			require.NoError(t, err)
			_, err = store.ListDue(ctx, base, 0)
			require.NoError(t, err)
			require.NoError(t, store.WakeDue(ctx, base))
			require.NoError(t, store.Reschedule(ctx, "k", 1, base))
			_, err = store.Expire(ctx, "k", 1, base, base)
			require.NoError(t, err)
			require.NoError(t, store.Resolve(ctx, "k", 1, base))
			_, err = store.Flag(ctx, "k", 1, "why")
			require.NoError(t, err)
			require.NoError(t, store.ClearFlag(ctx, "k", 1))
			_, err = store.DeleteObsolete(ctx, "k", 1)
			require.NoError(t, err)
			require.NoError(t, store.Delete(ctx, "k"))
			_, err = store.ListByState(ctx, retryqueue.StateExpired, 5)
			require.NoError(t, err)
			_, err = store.Requeue(ctx, base, "a", "b")
			require.NoError(t, err)
			_, err = store.PurgeExpired(ctx)
			require.NoError(t, err)

			for _, c := range fake.Calls() {
				if d == sqldb.DialectPostgres {
					require.NotContains(t, c.Query, "?", c.Query)
					require.Contains(t, c.Query, `"retry_items"`, c.Query)
					require.Len(t, numberedParamRE.FindAllString(c.Query, -1), len(c.Args), c.Query)
				} else {
					require.NotRegexp(t, numberedParamRE, c.Query)
					require.Contains(t, c.Query, "`retry_items`", c.Query)
					require.Equal(t, strings.Count(c.Query, "?"), len(c.Args), c.Query)
				}
			}
		})
	}
}

// TestMarkInsertsOrReads covers both outcomes of the insert: a new row is
// returned without a read, a taken key (ON CONFLICT on PostgreSQL, a
// duplicate-key error on MySQL) is read back.
func TestMarkInsertsOrReads(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			in := retryqueue.Item[payload]{Key: "k", Generation: 7, State: retryqueue.StateExpired, Attempts: 4, Flagged: true,
				Since: base.Local(), NextAttemptAt: base.Add(time.Hour), Payload: payload{Note: "new"}}

			store, fake, _ := newStore(t, d, nil)
			got, err := store.Mark(t.Context(), in)
			require.NoError(t, err)
			require.Equal(t, retryqueue.Item[payload]{Key: "k", Generation: 7, State: retryqueue.StatePending, Since: base,
				NextAttemptAt: base.Add(time.Hour), Payload: payload{Note: "new"}}, got)
			require.Len(t, fake.Calls(), 1)

			store, fake, _ = newStore(t, d, func(q string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(q, "INSERT") {
					if d == sqldb.DialectMySQL {
						return testhelpers.FakeSQLReply{Err: duplicate(d)}
					}
					return testhelpers.FakeSQLReply{Affected: 0}
				}
				return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{row("k", retryqueue.StatePending)}}
			})
			got, err = store.Mark(t.Context(), in)
			require.NoError(t, err)
			require.Equal(t, retryqueue.Item[payload]{Key: "k", Generation: 3, State: retryqueue.StatePending, Since: base, Attempts: 2,
				NextAttemptAt: base.Add(time.Minute), Flagged: true, FlagReason: "why", Payload: payload{Note: "stored"}}, got)
			calls := fake.Calls()
			require.Len(t, calls, 2)
			if d == sqldb.DialectMySQL {
				require.Contains(t, calls[1].Query, "LOCK IN SHARE MODE", "the read-back is a locking read on MySQL")
			}
		})
	}
}

// TestMarkErrors covers a failing insert, a vanishing item and a too long key.
func TestMarkErrors(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBoom}
	})
	_, err := store.Mark(t.Context(), retryqueue.Item[payload]{Key: "k"})
	require.ErrorIs(t, err, errBoom)

	// A duplicate-key error is an error on PostgreSQL, where ON CONFLICT
	// absorbs the conflict.
	store, _, _ = newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: duplicate(sqldb.DialectPostgres)}
	})
	_, err = store.Mark(t.Context(), retryqueue.Item[payload]{Key: "k"})
	require.Error(t, err)

	store, fake, _ := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "INSERT") {
			return testhelpers.FakeSQLReply{Affected: 0}
		}
		return testhelpers.FakeSQLReply{Columns: columns}
	})
	_, err = store.Mark(t.Context(), retryqueue.Item[payload]{Key: "k"})
	require.Error(t, err)
	require.Len(t, fake.Calls(), 4, "two insert-then-read rounds")

	_, err = store.Mark(t.Context(), retryqueue.Item[payload]{Key: strings.Repeat("ü", sqldb.MaxKeyLength+1)})
	require.ErrorIs(t, err, sqldb.ErrValueTooLong)
	_, err = store.Reopen(t.Context(), retryqueue.Item[payload]{Key: strings.Repeat("k", sqldb.MaxKeyLength+1)})
	require.ErrorIs(t, err, sqldb.ErrValueTooLong)
}

// TestReopenFallsBackToMark asserts an unmatched reopen marks the item.
func TestReopenFallsBackToMark(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "UPDATE") {
			return testhelpers.FakeSQLReply{Affected: 0}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	got, err := store.Reopen(t.Context(), retryqueue.Item[payload]{Key: "k", Generation: 2})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Generation)
	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.True(t, strings.HasPrefix(calls[1].Query, "INSERT"))
}

// TestMoveOutcomes covers the look-ups and the rename of Move.
func TestMoveOutcomes(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name             string
				from, to         string
				fromFound, taken bool
				update           testhelpers.FakeSQLReply
				want             error
				calls            int
			}{
				{name: "missing source", from: "a", to: "b", taken: true, want: retryqueue.ErrItemNotFound, calls: 1},
				{name: "onto itself", from: "a", to: "a", fromFound: true, calls: 1},
				{name: "taken target", from: "a", to: "b", fromFound: true, taken: true, want: retryqueue.ErrItemExists, calls: 2},
				{name: "moved", from: "a", to: "b", fromFound: true, update: testhelpers.FakeSQLReply{Affected: 1}, calls: 3},
				{name: "raced away", from: "a", to: "b", fromFound: true, want: retryqueue.ErrItemNotFound, calls: 3},
				{name: "raced in", from: "a", to: "b", fromFound: true, update: testhelpers.FakeSQLReply{Err: duplicate(d)},
					want: retryqueue.ErrItemExists, calls: 3},
				{name: "failed", from: "a", to: "b", fromFound: true, update: testhelpers.FakeSQLReply{Err: errBoom}, want: errBoom, calls: 3},
				{name: "too long target", from: "a", to: strings.Repeat("k", sqldb.MaxKeyLength+1), fromFound: true,
					want: sqldb.ErrValueTooLong, calls: 1},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					store, fake, _ := newStore(t, d, func(q string, args []any) testhelpers.FakeSQLReply {
						if strings.HasPrefix(q, "UPDATE") {
							return tc.update
						}
						key := args[0]
						if b, ok := key.([]byte); ok {
							key = string(b)
						}
						if (key == tc.from && tc.fromFound) || (key == tc.to && tc.taken) {
							return testhelpers.FakeSQLReply{Columns: []string{"one"}, Rows: [][]driver.Value{{int64(1)}}}
						}
						return testhelpers.FakeSQLReply{Columns: []string{"one"}}
					})
					err := store.Move(t.Context(), tc.from, tc.to)
					if tc.want == nil {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, tc.want)
					}
					require.Len(t, fake.Calls(), tc.calls)
				})
			}
		})
	}
}

// TestWithTxRunsOnTransaction asserts every statement runs on the transaction
// the context carries, and on the database otherwise.
func TestWithTxRunsOnTransaction(t *testing.T) {
	t.Parallel()
	store, fake, db := newStore(t, sqldb.DialectPostgres, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "SELECT") {
			return testhelpers.FakeSQLReply{Columns: []string{"one"}, Rows: [][]driver.Value{{int64(1)}}}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	ctx := sqldb.WithTx(t.Context(), tx)
	require.NoError(t, store.Delete(ctx, "k"))
	_, err = store.Mark(ctx, retryqueue.Item[payload]{Key: "k"})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, store.Delete(t.Context(), "k"))

	calls := fake.Calls()
	require.Len(t, calls, 3)
	require.True(t, calls[0].InTx)
	require.True(t, calls[1].InTx)
	require.False(t, calls[2].InTx)
}

// TestPurgeExpired asserts the purge needs a retention, binds it in
// nanoseconds, and repeats while a chunk comes back full.
func TestPurgeExpired(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectMySQL, nil)
	n, err := store.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, fake.Calls(), "no retention, no I/O")

	affected := []int64{1000, 1000, 7}
	store, fake, _ = newStore(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply {
		n := affected[0]
		affected = affected[1:]
		return testhelpers.FakeSQLReply{Affected: n}
	}, sqldb.WithResolvedRetention(90*time.Second))
	n, err = store.PurgeExpired(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2007), n)
	calls := fake.Calls()
	require.Len(t, calls, 3)
	require.Equal(t, []any{(90 * time.Second).Nanoseconds()}, calls[0].Args)
	require.Contains(t, calls[0].Query, "UNIX_TIMESTAMP()", "the cutoff is the database clock")

	store, _, _ = newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBoom}
	}, sqldb.WithResolvedRetention(time.Hour))
	_, err = store.PurgeExpired(t.Context())
	require.ErrorIs(t, err, errBoom)
}

// TestRequeueChunksKeys asserts a long key list is split across statements
// and the counts are summed.
func TestRequeueChunksKeys(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectPostgres, func(_ string, args []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Affected: int64(len(args) - 2)}
	})
	keys := make([]string, 2500)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
	}
	n, err := store.Requeue(t.Context(), base, keys...)
	require.NoError(t, err)
	require.Equal(t, 2500, n)
	require.Len(t, fake.Calls(), 3)

	n, err = store.Requeue(t.Context(), base)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	last := fake.Calls()[3]
	require.NotContains(t, last.Query, " IN ", "no keys requeue every expired item")
}

// TestListDecodesRows asserts list results are decoded and the default limit
// applies.
func TestListDecodesRows(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{row("a", retryqueue.StatePending), row("b", retryqueue.StatePending)}}
	})
	got, err := store.ListDue(t.Context(), base, -1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "b", got[1].Key)
	require.Equal(t, []any{base.UnixNano(), int64(retryqueue.DefaultListLimit)}, fake.Calls()[0].Args)

	store, _, _ = newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		bad := row("a", retryqueue.StatePending)
		bad[10] = []byte("{")
		return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{bad}}
	})
	_, err = store.ListByState(t.Context(), retryqueue.StatePending, 1)
	require.Error(t, err, "an undecodable payload fails the list")
}

// TestEnsureSchema pins the DDL of each dialect.
func TestEnsureSchema(t *testing.T) {
	t.Parallel()
	store, fake, _ := newStore(t, sqldb.DialectMySQL, nil, sqldb.WithTableName("app.queue"))
	require.NoError(t, store.EnsureSchema(t.Context()))
	calls := fake.Calls()
	require.Len(t, calls, 1)
	ddl := calls[0].Query
	require.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS `app`.`queue`")
	require.Contains(t, ddl, "VARBINARY(1020)")
	for _, ix := range []string{"`queue_due_idx` (state, next_attempt_at, id)", "`queue_state_idx` (state, since, id)", "`queue_resolved_idx` (state, resolved_at)"} {
		require.Contains(t, ddl, ix)
	}

	store, fake, _ = newStore(t, sqldb.DialectPostgres, nil)
	require.NoError(t, store.EnsureSchema(t.Context()))
	var stmts []string
	for _, c := range fake.Calls() {
		require.True(t, c.InTx, "PostgreSQL DDL runs in one transaction")
		stmts = append(stmts, c.Query)
	}
	require.Contains(t, stmts[0], "pg_advisory_xact_lock")
	require.Contains(t, stmts[1], `CREATE TABLE IF NOT EXISTS "retry_items"`)
	require.Contains(t, stmts[1], `COLLATE "C"`)
	require.Len(t, stmts, 5)

	store, _, _ = newStore(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBoom}
	})
	require.ErrorIs(t, store.EnsureSchema(t.Context()), errBoom)
}

// TestCodecErrors asserts payload codec failures surface.
func TestCodecErrors(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, sqldb.DialectPostgres, nil, sqldb.WithCodec(failingCodec{}))
	_, err := store.Mark(t.Context(), retryqueue.Item[payload]{Key: "k"})
	require.ErrorIs(t, err, errBoom)
	_, err = store.Reopen(t.Context(), retryqueue.Item[payload]{Key: "k"})
	require.ErrorIs(t, err, errBoom)
}

type failingCodec struct{}

func (failingCodec) Serialize(any) ([]byte, error) { return nil, errBoom }
func (failingCodec) Deserialize([]byte, any) error { return errBoom }
