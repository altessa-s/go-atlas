// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/audit/storages/storagetest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var (
	errBoom         = errors.New("boom")
	numberedParamRE = regexp.MustCompile(`\$\d+`)
	base            = time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
)

func newStorage(t *testing.T, d sqldb.Dialect, respond func(string, []any) testhelpers.FakeSQLReply,
	opts ...sqldb.Option) (*sqldb.Storage, *testhelpers.FakeSQL) {
	t.Helper()
	db, fake := testhelpers.NewFakeSQL(t, respond)
	s, err := sqldb.New(db, d, opts...)
	require.NoError(t, err)
	return s, fake
}

func event(id string) *audit.Event {
	return &audit.Event{
		ID: id, Type: "auth", Action: "login", Timestamp: base,
		Actor:    audit.Actor{ID: "u1", Type: "user", Roles: []string{"admin"}, Metadata: map[string]any{"k": "v"}},
		Resource: audit.Resource{Type: "doc", ID: "d1", Changes: &audit.ResourceChanges{Fields: []string{"title"}}},
		Result:   audit.Result{Status: "success", Code: 200},
		Context:  audit.EventContext{RequestID: "req", TraceID: "trace"},
		Service:  audit.ServiceInfo{Name: "svc"},
		Duration: 1500 * time.Microsecond,
	}
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New(db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName("a.b.c"))
	require.ErrorIs(t, err, sqldb.ErrInvalidTableName)
}

// TestRoundTrip stores an event and feeds the bound payload back as the row a
// query reads: every field, the timestamp at full precision, survives.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	for _, d := range []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL} {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			var (
				mu      sync.Mutex
				payload driver.Value
			)
			s, fake := newStorage(t, d, func(query string, args []any) testhelpers.FakeSQLReply {
				mu.Lock()
				defer mu.Unlock()
				if strings.HasPrefix(query, "INSERT") {
					payload = args[len(args)-1]
					return testhelpers.FakeSQLReply{Affected: 1}
				}
				return testhelpers.FakeSQLReply{Columns: []string{"payload"}, Rows: [][]driver.Value{{payload}}}
			})
			want := event("e1")
			require.NoError(t, s.Store(t.Context(), want))
			insert := fake.Calls()[0]
			require.Equal(t, base.UnixMilli(), insert.Args[1], "ts_ms")

			var got []*audit.Event
			for e, err := range s.Query(t.Context(), &audit.Query{}) {
				require.NoError(t, err)
				got = append(got, e)
			}
			require.Equal(t, []*audit.Event{want}, got)
		})
	}
}

// TestInsertIsIdempotent pins the conflict clauses that make a replayed event
// a no-op without swallowing other errors.
func TestInsertIsIdempotent(t *testing.T) {
	t.Parallel()
	pg, fake := newStorage(t, sqldb.DialectPostgres, nil)
	require.NoError(t, pg.Store(t.Context(), event("e1")))
	q := fake.Calls()[0].Query
	require.Contains(t, q, "ON CONFLICT (id) DO NOTHING")
	require.NotContains(t, q, "?")
	require.Len(t, numberedParamRE.FindAllString(q, -1), 12)

	my, fake := newStorage(t, sqldb.DialectMySQL, nil)
	require.NoError(t, my.Store(t.Context(), event("e1")))
	q = fake.Calls()[0].Query
	require.Contains(t, q, "ON DUPLICATE KEY UPDATE id = id")
	require.NotContains(t, q, "IGNORE")
	require.Equal(t, []byte("e1"), fake.Calls()[0].Args[0], "binary columns get bytes")
}

// TestStoreBatchChunksInOneTransaction pins that a batch larger than the row
// limit is split into INSERTs of one transaction, and that a failing chunk
// rolls the whole batch back.
func TestStoreBatchChunksInOneTransaction(t *testing.T) {
	t.Parallel()
	batch := make([]*audit.Event, 5)
	for i := range batch {
		batch[i] = event(fmt.Sprintf("e%d", i))
	}

	s, fake := newStorage(t, sqldb.DialectPostgres, nil, sqldb.WithMaxBatchRows(2))
	require.NoError(t, s.StoreBatch(t.Context(), batch))
	calls := fake.Calls()
	require.Len(t, calls, 3)
	for i, rows := range []int{2, 2, 1} {
		require.True(t, calls[i].InTx)
		require.Len(t, calls[i].Args, rows*12)
	}
	require.Equal(t, 1, fake.Commits())

	var n int
	s, fake = newStorage(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		if n++; n == 2 {
			return testhelpers.FakeSQLReply{Err: errBoom}
		}
		return testhelpers.FakeSQLReply{Affected: 2}
	}, sqldb.WithMaxBatchRows(2))
	require.ErrorIs(t, s.StoreBatch(t.Context(), batch), errBoom)
	require.Zero(t, fake.Commits())
	require.Equal(t, 1, fake.Rollbacks())

	s, fake = newStorage(t, sqldb.DialectPostgres, nil)
	require.NoError(t, s.StoreBatch(t.Context(), batch))
	require.Len(t, fake.Calls(), 1, "a batch within the limit is one statement")
	require.False(t, fake.Calls()[0].InTx)
	require.NoError(t, s.StoreBatch(t.Context(), nil))
	require.Len(t, fake.Calls(), 1, "an empty batch writes nothing")
}

func TestStoreValueTooLong(t *testing.T) {
	t.Parallel()
	long := event("long-actor")
	long.Actor.ID = strings.Repeat("a", sqldb.MaxFilterBytes+1)
	fits := event("fits")
	fits.Actor.ID = strings.Repeat("a", sqldb.MaxFilterBytes)
	for name, e := range map[string]*audit.Event{"id": event(strings.Repeat("ü", sqldb.MaxIDLength+1)), "filter field": long} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, fake := newStorage(t, sqldb.DialectPostgres, nil)
			require.ErrorIs(t, s.StoreBatch(t.Context(), []*audit.Event{event("ok"), e}), sqldb.ErrValueTooLong)
			require.Empty(t, fake.Calls(), "nothing of the batch is written")
		})
	}
	s, _ := newStorage(t, sqldb.DialectPostgres, nil)
	require.NoError(t, s.Store(t.Context(), fits))
}

// TestMaxBatchRowsIsCapped pins that one INSERT never exceeds PostgreSQL's
// 65535 bind parameters, whatever WithMaxBatchRows asks for.
func TestMaxBatchRowsIsCapped(t *testing.T) {
	t.Parallel()
	batch := make([]*audit.Event, 6000)
	for i := range batch {
		batch[i] = event(fmt.Sprintf("e%d", i))
	}
	s, fake := newStorage(t, sqldb.DialectPostgres, nil, sqldb.WithMaxBatchRows(10_000))
	require.NoError(t, s.StoreBatch(t.Context(), batch))
	calls := fake.Calls()
	require.Len(t, calls, 2)
	for _, c := range calls {
		require.LessOrEqual(t, len(c.Args), 65535)
		require.True(t, c.InTx)
	}
}

// TestQueryStatement pins the filter, cursor, order and limit of a query.
func TestQueryStatement(t *testing.T) {
	t.Parallel()
	s, fake := newStorage(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"payload"}}
	})
	start, end := base, base.Add(time.Hour)
	for range s.Query(t.Context(), &audit.Query{
		StartTime: &start, EndTime: &end, ActorID: "u1", Status: "success", Limit: 10, SortOrder: audit.SortOrderAsc,
		Cursor: &audit.Cursor{Timestamp: base.Add(time.Minute), ID: "e5"},
	}) {
		t.Fatal("no rows expected")
	}
	c := fake.Calls()[0]
	require.Equal(t, `SELECT payload FROM "audit_events" WHERE ts_ms >= $1 AND ts_ms <= $2 AND actor_id = $3 AND status = $4`+
		` AND (ts_ms > $5 OR (ts_ms = $6 AND id > $7)) ORDER BY ts_ms ASC, id ASC LIMIT $8`, c.Query)
	ms := base.Add(time.Minute).UnixMilli()
	require.Equal(t, []any{start.UnixMilli(), end.UnixMilli(), "u1", "success", ms, ms, "e5", int64(10)}, c.Args)

	_, err := s.Count(t.Context(), &audit.Query{ActorID: "u1", Limit: 1, Cursor: &audit.Cursor{ID: "x"}})
	require.Error(t, err, "the fake returns no count row")
	require.Equal(t, `SELECT COUNT(*) FROM "audit_events" WHERE actor_id = $1`, fake.Calls()[1].Query)
}

// TestQueryStopsEarly pins that breaking out of the iteration closes the
// result set.
func TestQueryStopsEarly(t *testing.T) {
	t.Parallel()
	row := []driver.Value{`{"id":"e1"}`}
	s, fake := newStorage(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: []string{"payload"}, Rows: [][]driver.Value{row, row, row}}
	})
	for range s.Query(t.Context(), &audit.Query{}) {
		break
	}
	require.Equal(t, 1, fake.RowsClosed())
	require.Equal(t, "SELECT payload FROM `audit_events` ORDER BY ts_ms DESC, id DESC", fake.Calls()[0].Query)
}

func TestErrorsAreReported(t *testing.T) {
	t.Parallel()
	s, _ := newStorage(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBoom}
	})
	require.ErrorIs(t, s.Store(t.Context(), event("e1")), errBoom)
	for _, err := range s.Query(t.Context(), &audit.Query{}) {
		require.ErrorIs(t, err, errBoom)
	}
	_, err := s.Count(t.Context(), &audit.Query{})
	require.ErrorIs(t, err, errBoom)
	require.ErrorIs(t, s.EnsureSchema(t.Context()), errBoom)
	require.NoError(t, s.Close(t.Context()))
}

func TestEnsureSchema(t *testing.T) {
	t.Parallel()
	pg, fake := newStorage(t, sqldb.DialectPostgres, nil)
	require.NoError(t, pg.EnsureSchema(t.Context()))
	calls := fake.Calls()
	require.Len(t, calls, 7, "advisory lock, table, five indexes")
	require.Contains(t, calls[1].Query, `id VARCHAR(255) COLLATE "C" PRIMARY KEY`)
	require.Contains(t, calls[2].Query, `"audit_events_ts_idx" ON "audit_events" (ts_ms, id)`)

	my, fake := newStorage(t, sqldb.DialectMySQL, nil)
	require.NoError(t, my.EnsureSchema(t.Context()))
	q := fake.Calls()[0].Query
	require.NotRegexp(t, `\b(VARCHAR|TEXT)\b`, q)
	require.Contains(t, q, "INDEX `audit_events_actor_idx` (actor_id, ts_ms, id)")
}

func TestNilQueryStatements(t *testing.T) {
	t.Parallel()
	s, fake := newStorage(t, sqldb.DialectPostgres, func(query string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(query, "SELECT COUNT(") {
			return testhelpers.FakeSQLReply{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(0)}}}
		}
		return testhelpers.FakeSQLReply{Columns: []string{"payload"}}
	})
	for e, err := range s.Query(t.Context(), nil) {
		require.NoError(t, err)
		require.FailNow(t, "unexpected row", "%v", e)
	}
	count, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	require.Zero(t, count)
	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.Equal(t, `SELECT payload FROM "audit_events" ORDER BY ts_ms DESC, id DESC`, calls[0].Query)
	require.Equal(t, `SELECT COUNT(*) FROM "audit_events"`, calls[1].Query)
}

func TestStoreBatchNilEvent(t *testing.T) {
	t.Parallel()
	s, fake := newStorage(t, sqldb.DialectPostgres, nil)
	storagetest.NilBatchEvent(t, s)
	require.Empty(t, fake.Calls())
}

func TestRoundTripPreservesLargeIntegers(t *testing.T) {
	t.Parallel()

	const large = "9007199254740993"
	var (
		mu      sync.Mutex
		payload driver.Value
	)
	s, _ := newStorage(t, sqldb.DialectPostgres, func(query string, args []any) testhelpers.FakeSQLReply {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(query, "INSERT") {
			payload = args[len(args)-1]
			return testhelpers.FakeSQLReply{Affected: 1}
		}
		return testhelpers.FakeSQLReply{Columns: []string{"payload"}, Rows: [][]driver.Value{{payload}}}
	})

	in := event("e-large")
	in.Metadata = map[string]any{"seq": json.Number(large)}
	in.Actor.Metadata = map[string]any{"actor_seq": json.Number(large)}
	in.Resource.Attributes = map[string]any{"res_seq": json.Number(large)}
	in.Resource.Changes = &audit.ResourceChanges{
		Before: map[string]any{"seq": json.Number(large)},
		After:  map[string]any{"seq": json.Number(large)},
	}
	require.NoError(t, s.Store(t.Context(), in))

	var got []*audit.Event
	for e, err := range s.Query(t.Context(), &audit.Query{}) {
		require.NoError(t, err)
		got = append(got, e)
	}
	require.Len(t, got, 1)

	encoded, err := json.Marshal(got[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "9007199254740992")
	require.Contains(t, string(encoded), `"seq":`+large)
	require.Contains(t, string(encoded), `"actor_seq":`+large)
	require.Contains(t, string(encoded), `"res_seq":`+large)
}
