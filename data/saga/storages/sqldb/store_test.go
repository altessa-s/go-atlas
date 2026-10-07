// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/sqldb"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

var (
	errBoom         = errors.New("boom")
	numberedParamRE = regexp.MustCompile(`\$\d+`)
	dialects        = []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL}
	base            = time.Unix(1_700_000_000, 0).UTC()
)

// columns is the scan order of the instances table.
var columns = []string{"id", "definition", "status", "stage", "pending_steps", "data", "steps", "created_at", "updated_at",
	"deadline", "lease_owner", "lease_until", "version", "last_error"}

func newStore(t *testing.T, d sqldb.Dialect, respond func(query string, args []any) testhelpers.FakeSQLReply) (*sqldb.Store, *testhelpers.FakeSQL) {
	t.Helper()
	db, fake := testhelpers.NewFakeSQL(t, respond)
	store, err := sqldb.New(db, d)
	require.NoError(t, err)
	return store, fake
}

func fullInstance() *saga.Instance {
	return &saga.Instance{
		ID:           "order-ü",
		Definition:   "place-order",
		Status:       saga.StatusRunning,
		Stage:        2,
		PendingSteps: []int{0, 3},
		Data:         []byte{0, 1, 0xff},
		Steps: []saga.StepRecord{{Name: "reserve", Status: saga.StepCompleted, Attempts: 1, StartedAt: base,
			FinishedAt: base.Add(time.Second)}},
		CreatedAt:  base,
		UpdatedAt:  base.Add(time.Minute),
		Deadline:   base.Add(time.Hour),
		LeaseOwner: "owner",
		LeaseUntil: base.Add(123456789 * time.Nanosecond),
		Version:    4,
		LastError:  "boom",
	}
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)

	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.Error(t, err)
	_, err = sqldb.New(db, "oracle")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	for _, name := range []string{"t; DROP TABLE x", "a.b.c", "1saga", `"quoted"`, "sa ga", strings.Repeat("t", 64)} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName(name))
		require.ErrorIs(t, err, sqldb.ErrInvalidTableName, name)
	}
	_, err = sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTableName("app.saga"))
	require.NoError(t, err)
}

// TestPlaceholderStyle pins that every statement uses the dialect's
// placeholders and quoting.
func TestPlaceholderStyle(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			store, fake := newStore(t, d, nil)
			ctx := t.Context()
			require.NoError(t, store.Create(ctx, fullInstance()))
			_, _ = store.Get(ctx, "a")
			require.NoError(t, store.Update(ctx, fullInstance()))
			_, err := store.FetchRecoverable(ctx, base, 0)
			require.NoError(t, err)
			_, err = store.FetchRecoverable(ctx, base, 5)
			require.NoError(t, err)
			require.NoError(t, store.Delete(ctx, "a"))

			for _, c := range fake.Calls() {
				if d == sqldb.DialectPostgres {
					require.NotContains(t, c.Query, "?", c.Query)
					require.NotContains(t, c.Query, "`", c.Query)
					require.Len(t, numberedParamRE.FindAllString(c.Query, -1), len(c.Args), c.Query)
				} else {
					require.NotRegexp(t, numberedParamRE, c.Query)
					require.NotContains(t, c.Query, `"saga_instances"`, c.Query)
					require.Equal(t, strings.Count(c.Query, "?"), len(c.Args), c.Query)
				}
			}
		})
	}
}

// TestRoundTripEncoding feeds the values Create binds back as the row Get
// scans, so encoding and decoding stay symmetric on both dialects.
func TestRoundTripEncoding(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			var (
				mu  sync.Mutex
				row []driver.Value
			)
			store, _ := newStore(t, d, func(query string, args []any) testhelpers.FakeSQLReply {
				mu.Lock()
				defer mu.Unlock()
				if strings.HasPrefix(query, "INSERT") {
					row = make([]driver.Value, len(args))
					for i, a := range args {
						row[i] = a
					}
					return testhelpers.FakeSQLReply{Affected: 1}
				}
				return testhelpers.FakeSQLReply{Columns: columns, Rows: [][]driver.Value{row}}
			})
			for _, inst := range []*saga.Instance{fullInstance(), {ID: "zero"}} {
				require.NoError(t, store.Create(t.Context(), inst))
				got, err := store.Get(t.Context(), inst.ID)
				require.NoError(t, err)
				want := inst.Clone()
				if want.Data == nil {
					want.Data = []byte{}
				}
				require.Equal(t, want, got)
			}
		})
	}
}

// TestTextArgs pins how strings are bound: as strings on PostgreSQL, as bytes
// on MySQL, whose columns are binary.
func TestTextArgs(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			store, fake := newStore(t, d, nil)
			require.NoError(t, store.Delete(t.Context(), "a"))
			if d == sqldb.DialectPostgres {
				require.Equal(t, []any{"a"}, fake.Calls()[0].Args)
			} else {
				require.Equal(t, []any{[]byte("a")}, fake.Calls()[0].Args)
			}
		})
	}
}

func TestCreateConflictPostgres(t *testing.T) {
	t.Parallel()
	store, fake := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Affected: 0}
	})
	require.ErrorIs(t, store.Create(t.Context(), fullInstance()), sagaerrs.ErrInstanceExists)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	require.Contains(t, calls[0].Query, "ON CONFLICT (id) DO NOTHING")
}

func TestCreateFailureMySQL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		exists bool
		want   error
	}{
		{name: "duplicate", exists: true, want: sagaerrs.ErrInstanceExists},
		{name: "other error", exists: false, want: errBoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, fake := newStore(t, sqldb.DialectMySQL, func(query string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(query, "INSERT") {
					return testhelpers.FakeSQLReply{Err: errBoom}
				}
				reply := testhelpers.FakeSQLReply{Columns: []string{"1"}}
				if tc.exists {
					reply.Rows = [][]driver.Value{{int64(1)}}
				}
				return reply
			})
			require.ErrorIs(t, store.Create(t.Context(), fullInstance()), tc.want)
			calls := fake.Calls()
			require.Len(t, calls, 2)
			require.NotContains(t, calls[0].Query, "IGNORE")
		})
	}
}

func TestCreateIDTooLong(t *testing.T) {
	t.Parallel()
	store, fake := newStore(t, sqldb.DialectPostgres, nil)
	require.ErrorIs(t, store.Create(t.Context(), &saga.Instance{ID: strings.Repeat("ü", sqldb.MaxIDLength+1)}), sqldb.ErrValueTooLong)
	require.Empty(t, fake.Calls())
	require.NoError(t, store.Create(t.Context(), &saga.Instance{ID: strings.Repeat("ü", sqldb.MaxIDLength)}))
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		affected int64
		exists   bool
		want     error
	}{
		{name: "success", affected: 1},
		{name: "conflict", affected: 0, exists: true, want: sagaerrs.ErrVersionConflict},
		{name: "not found", affected: 0, exists: false, want: sagaerrs.ErrInstanceNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, fake := newStore(t, sqldb.DialectPostgres, func(query string, _ []any) testhelpers.FakeSQLReply {
				if strings.HasPrefix(query, "UPDATE") {
					return testhelpers.FakeSQLReply{Affected: tc.affected}
				}
				reply := testhelpers.FakeSQLReply{Columns: []string{"1"}}
				if tc.exists {
					reply.Rows = [][]driver.Value{{int64(1)}}
				}
				return reply
			})
			inst := fullInstance()
			err := store.Update(t.Context(), inst)
			calls := fake.Calls()
			update := calls[0].Args
			require.Equal(t, int64(5), update[10], "new version")
			require.Equal(t, int64(4), update[12], "expected version")
			if tc.want == nil {
				require.NoError(t, err)
				require.Equal(t, int64(5), inst.Version)
				require.Len(t, calls, 1)
				return
			}
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, int64(4), inst.Version, "a failed Update must not change the version")
			require.Len(t, calls, 2)
		})
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()
	store, _ := newStore(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: columns}
	})
	_, err := store.Get(t.Context(), "missing")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

func TestFetchRecoverable(t *testing.T) {
	t.Parallel()
	now := base.Add(500 * time.Millisecond)
	store, fake := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Columns: columns}
	})

	_, err := store.FetchRecoverable(t.Context(), now, 0)
	require.NoError(t, err)
	_, err = store.FetchRecoverable(t.Context(), now, 7)
	require.NoError(t, err)

	calls := fake.Calls()
	require.Len(t, calls, 2)
	require.NotContains(t, calls[0].Query, "LIMIT")
	require.Equal(t, []any{"RUNNING", "COMPENSATING", now.UnixNano(), "COMPENSATING", now.UnixNano()}, calls[0].Args)
	require.Contains(t, calls[1].Query, "LIMIT $6")
	require.Equal(t, int64(7), calls[1].Args[5])
	require.Equal(t, 2, fake.RowsClosed())
}

func TestErrorsAreWrapped(t *testing.T) {
	t.Parallel()
	store, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBoom}
	})
	ctx := t.Context()
	require.ErrorIs(t, store.Create(ctx, fullInstance()), errBoom)
	_, err := store.Get(ctx, "a")
	require.ErrorIs(t, err, errBoom)
	require.ErrorIs(t, store.Update(ctx, fullInstance()), errBoom)
	_, err = store.FetchRecoverable(ctx, base, 0)
	require.ErrorIs(t, err, errBoom)
	require.ErrorIs(t, store.Delete(ctx, "a"), errBoom)
	require.ErrorIs(t, store.EnsureSchema(ctx), errBoom)
}

func TestEnsureSchemaPostgres(t *testing.T) {
	t.Parallel()
	store, fake := newStore(t, sqldb.DialectPostgres, nil)
	require.NoError(t, store.EnsureSchema(t.Context()))

	calls := fake.Calls()
	require.Len(t, calls, 4)
	require.Contains(t, calls[0].Query, "pg_advisory_xact_lock")
	require.Contains(t, calls[1].Query, `CREATE TABLE IF NOT EXISTS "saga_instances"`)
	require.Contains(t, calls[1].Query, `VARCHAR(255) COLLATE "C" PRIMARY KEY`)
	require.Contains(t, calls[2].Query, `"saga_instances_lease_idx" ON "saga_instances" (status, lease_until)`)
	require.Contains(t, calls[3].Query, `"saga_instances_deadline_idx" ON "saga_instances" (status, deadline)`)
	for _, c := range calls {
		require.True(t, c.InTx, c.Query)
	}
	require.Equal(t, 1, fake.Commits())
}

// TestEnsureSchemaMySQLBinaryTypes pins that the MySQL schema declares every
// string column as a binary type, independent of character set and collation.
func TestEnsureSchemaMySQLBinaryTypes(t *testing.T) {
	t.Parallel()
	store, fake := newStore(t, sqldb.DialectMySQL, nil)
	require.NoError(t, store.EnsureSchema(t.Context()))

	calls := fake.Calls()
	require.Len(t, calls, 1)
	q := calls[0].Query
	require.NotRegexp(t, `\b(VARCHAR|TEXT|MEDIUMTEXT|LONGTEXT)\b`, q)
	require.NotContains(t, q, "COLLATE")
	require.NotContains(t, q, "CHARACTER SET")
	for _, want := range []string{"id            VARBINARY(1020) NOT NULL PRIMARY KEY", "status        VARBINARY(64)",
		"INDEX `saga_instances_lease_idx` (status, lease_until)", "INDEX `saga_instances_deadline_idx` (status, deadline)"} {
		require.Contains(t, q, want)
	}
}
