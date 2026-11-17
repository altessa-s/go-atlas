// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb_test

import (
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var dialects = []sqldb.Dialect{sqldb.DialectPostgres, sqldb.DialectMySQL}
var errSQL = errors.New("SQL failure")

func newStore(tb testing.TB, d sqldb.Dialect, respond func(string, []any) testhelpers.FakeSQLReply) (*sqldb.Storage, *testhelpers.FakeSQL) {
	tb.Helper()
	db, fake := testhelpers.NewFakeSQL(tb, respond)
	s, err := sqldb.New(db, d, sqldb.WithPurgeInterval(0))
	require.NoError(tb, err)
	return s, fake
}

func TestNewValidation(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err := sqldb.New(nil, sqldb.DialectPostgres)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	_, err = sqldb.New(db, "mariadb")
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	for _, name := range []string{"bad name", "a.b.c", "1table", "a;drop", strings.Repeat("a", 64)} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTableName(name))
		require.ErrorIs(t, err, sqldb.ErrInvalidTableName)
	}
	for _, ttl := range []time.Duration{0, -time.Second} {
		_, err = sqldb.New(db, sqldb.DialectPostgres, sqldb.WithTTL(ttl))
		require.NoError(t, err, "non-positive TTL options preserve the default")
	}
	_, err = sqldb.New(db, sqldb.DialectMySQL, sqldb.WithTableName("app.keys"))
	require.NoError(t, err)
	require.Empty(t, fake.Calls(), "construction must perform no I/O")
}

func TestAttemptLock(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		for _, mode := range []string{"insert", "expired", "held", "read raced delete"} {
			t.Run(string(d)+"/"+mode, func(t *testing.T) {
				t.Parallel()
				calls := 0
				s, fake := newStore(t, d, func(q string, _ []any) testhelpers.FakeSQLReply {
					calls++
					if strings.HasPrefix(q, "SELECT") {
						r := testhelpers.FakeSQLReply{Columns: []string{"val"}}
						if mode == "held" {
							r.Rows = [][]driver.Value{{[]byte{0, 255, 1}}}
						}
						return r
					}
					if mode == "insert" || mode == "expired" && strings.HasPrefix(q, "UPDATE") || mode == "read raced delete" && calls == 4 {
						return testhelpers.FakeSQLReply{Affected: 1}
					}
					return testhelpers.FakeSQLReply{}
				})
				val := []byte{0, 255, 1}
				ok, existing, tok, err := s.AttemptLockWithTTL(t.Context(), "k", val, time.Minute)
				require.NoError(t, err)
				require.Equal(t, mode != "held", ok)
				if ok {
					require.Nil(t, existing)
					require.Equal(t, val, tok)
					tok[0] = 2
					require.Equal(t, byte(0), val[0], "tokens must not alias input")
				} else {
					require.Equal(t, val, existing, "identical competing bytes must still lose")
					require.Nil(t, tok)
					require.Contains(t, fake.Calls()[2].Query, "FOR UPDATE")
				}
				var ttl any = float64(60)
				if d == sqldb.DialectMySQL {
					ttl = int64(time.Minute.Microseconds())
				}
				require.Equal(t, ttl, fake.Calls()[0].Args[2])
			})
		}
	}
}

func TestTTLAndCAS(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			s, err := sqldb.New(db, d, sqldb.WithTTL(time.Hour), sqldb.WithPurgeInterval(0))
			require.NoError(t, err)
			_, _, _, err = s.AttemptLockWithTTL(t.Context(), "k", []byte("v"), -time.Second)
			require.NoError(t, err)
			require.NoError(t, s.Complete(t.Context(), "k", []byte("done"), []byte("v")))
			token, err := s.Steal(t.Context(), "k", []byte("done"), []byte("new"))
			require.NoError(t, err)
			require.Equal(t, []byte("new"), token)
			require.NoError(t, s.Release(t.Context(), "k", token))
			require.NoError(t, s.Complete(t.Context(), "k", nil, nil))
			require.NoError(t, s.Delete(t.Context(), "k"))
			n, err := s.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Equal(t, int64(1), n)
			c := fake.Calls()
			require.Equal(t, c[0].Args[2], c[1].Args[1], "Complete resets the configured result TTL")
			require.Equal(t, c[0].Args[2], c[2].Args[1], "Steal resets the configured TTL")
			require.Equal(t, []byte("v"), c[1].Args[3])
			require.Equal(t, []byte("done"), c[2].Args[3])
			require.Equal(t, []byte("new"), c[3].Args[1])
			require.NotNil(t, c[4].Args[1], "nil bytes must bind an empty blob, not SQL NULL")
			require.Contains(t, c[1].Query, "revision = revision + 1")
			require.Contains(t, c[1].Query, "AND val = ")
			require.Contains(t, c[1].Query, "AND expires_at > ")
			require.Contains(t, c[2].Query, "AND expires_at > ")
			require.Contains(t, c[3].Query, "AND expires_at > ")
			if d == sqldb.DialectPostgres {
				require.Contains(t, c[0].Query, "ON CONFLICT")
				require.Contains(t, c[4].Query, "DO UPDATE")
				require.Contains(t, c[6].Query, "SKIP LOCKED")
			} else {
				require.Contains(t, c[0].Query, "INSERT IGNORE")
				require.Contains(t, c[4].Query, "ON DUPLICATE KEY UPDATE")
				require.Contains(t, c[6].Query, "LIMIT ?")
			}
			numbered := regexp.MustCompile(`\$\d+`)
			for _, call := range c {
				if d == sqldb.DialectPostgres {
					require.NotContains(t, call.Query, "?")
					require.Len(t, numbered.FindAllString(call.Query, -1), len(call.Args))
					require.Contains(t, call.Query, `"idempotency_keys"`)
				} else {
					require.Equal(t, strings.Count(call.Query, "?"), len(call.Args))
					require.Contains(t, call.Query, "`idempotency_keys`")
				}
			}
		})
	}
}

func TestCASMiss(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t, sqldb.DialectMySQL, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{} })
	require.ErrorIs(t, s.Complete(t.Context(), "k", nil, []byte("stale")), storages.ErrLockStolen)
	_, err := s.Steal(t.Context(), "k", []byte("stale"), []byte("new"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
	require.ErrorIs(t, s.Release(t.Context(), "k", []byte("stale")), storages.ErrLockStolen)
	require.NoError(t, s.Complete(t.Context(), "k", nil, nil))
}

func TestKeyValidation(t *testing.T) {
	t.Parallel()
	s, fake := newStore(t, sqldb.DialectPostgres, nil)
	for _, tc := range []struct {
		key string
		err error
	}{{"", storages.ErrEmptyKey}, {strings.Repeat("ü", 256), sqldb.ErrValueTooLong}} {
		_, _, _, err := s.AttemptLock(t.Context(), tc.key, nil)
		require.ErrorIs(t, err, tc.err)
		require.ErrorIs(t, s.Complete(t.Context(), tc.key, nil, nil), tc.err)
		_, err = s.Steal(t.Context(), tc.key, nil, nil)
		require.ErrorIs(t, err, tc.err)
		require.ErrorIs(t, s.Release(t.Context(), tc.key, nil), tc.err)
		require.ErrorIs(t, s.Delete(t.Context(), tc.key), tc.err)
	}
	require.ErrorIs(t, s.Release(t.Context(), "k", nil), storages.ErrMissingLockState)
	require.Empty(t, fake.Calls())
	require.NoError(t, s.Delete(t.Context(), strings.Repeat("ü", 255)))
}

func TestWithTx(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	s, err := sqldb.New(db, sqldb.DialectPostgres, sqldb.WithPurgeInterval(0))
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ctx := sqldb.WithTx(t.Context(), tx)
	_, _, _, err = s.AttemptLock(ctx, "k", []byte("v"))
	require.NoError(t, err)
	require.NoError(t, s.Complete(ctx, "k", []byte("done"), []byte("v")))
	require.NoError(t, s.Complete(ctx, "k", []byte("done"), nil))
	_, err = s.Steal(ctx, "k", []byte("done"), []byte("new"))
	require.NoError(t, err)
	require.NoError(t, s.Release(ctx, "k", []byte("new")))
	require.NoError(t, s.Delete(ctx, "k"))
	_, err = s.PurgeExpired(ctx)
	require.NoError(t, err)
	for _, c := range fake.Calls() {
		require.True(t, c.InTx, c.Query)
	}
	require.Zero(t, fake.Commits(), "the caller owns transaction completion")
	require.NoError(t, tx.Rollback())
	require.NoError(t, s.Delete(sqldb.WithTx(t.Context(), nil), "k"))
	require.False(t, fake.Calls()[7].InTx)
}

func TestErrors(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			t.Parallel()
			calls := 0
			s, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply {
				calls++
				if calls == failAt {
					return testhelpers.FakeSQLReply{Err: errSQL}
				}
				return testhelpers.FakeSQLReply{}
			})
			_, _, _, err := s.AttemptLock(t.Context(), "k", []byte("v"))
			require.ErrorIs(t, err, errSQL)
		})
	}
	s, _ := newStore(t, sqldb.DialectPostgres, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: errSQL} })
	require.ErrorIs(t, s.Complete(t.Context(), "k", nil, nil), errSQL)
	require.ErrorIs(t, s.Complete(t.Context(), "k", nil, []byte("v")), errSQL)
	_, err := s.Steal(t.Context(), "k", nil, nil)
	require.ErrorIs(t, err, errSQL)
	require.ErrorIs(t, s.Release(t.Context(), "k", []byte("v")), errSQL)
	require.ErrorIs(t, s.Delete(t.Context(), "k"), errSQL)
	_, err = s.PurgeExpired(t.Context())
	require.ErrorIs(t, err, errSQL)
	require.ErrorIs(t, s.EnsureSchema(t.Context()), errSQL)
}

func TestEnsureSchema(t *testing.T) {
	t.Parallel()
	for _, d := range dialects {
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()
			s, fake := newStore(t, d, nil)
			require.NoError(t, s.EnsureSchema(t.Context()))
			c := fake.Calls()
			if d == sqldb.DialectPostgres {
				require.Len(t, c, 3)
				require.Contains(t, c[0].Query, "pg_advisory_xact_lock")
				require.Contains(t, c[1].Query, `VARCHAR(255) COLLATE "C" PRIMARY KEY`)
				require.Contains(t, c[2].Query, "CREATE INDEX IF NOT EXISTS")
				require.Equal(t, 1, fake.Commits())
			} else {
				require.Len(t, c, 1)
				require.Contains(t, c[0].Query, "VARBINARY(1020)")
				require.Contains(t, c[0].Query, "LONGBLOB")
				require.Contains(t, c[0].Query, "INDEX `idempotency_keys_expires_idx` (expires_at)")
			}
		})
	}
}
