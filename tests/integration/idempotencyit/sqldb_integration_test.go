// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/storagetest"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	idemfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
)

// sqlTarget is one database/sql server of the docker stack, with its dialect
// in the storage's own type.
type sqlTarget struct {
	livebackend.SQLTarget
	name    string
	dialect sqldb.Dialect
}

func sqlTargets() []sqlTarget {
	var out []sqlTarget
	for _, t := range livebackend.SQLTargets() {
		out = append(out, sqlTarget{SQLTarget: t, name: t.Name, dialect: sqldb.Dialect(t.Dialect)})
	}
	return out
}

func sqlBackends() []backend {
	var result []backend
	for _, target := range sqlTargets() {
		result = append(result, backend{name: target.name, open: func(tb testing.TB, ttl time.Duration) storages.Storage {
			tb.Helper()
			_, _, s := target.open(tb, ttl)
			return s
		}})
	}
	return result
}

// open follows sagait: a unique disposable table, two schema calls to check
// idempotence, and a skip if the local backend is unavailable.
func (target sqlTarget) open(tb testing.TB, ttl time.Duration, opts ...sqldb.Option) (*sql.DB, string, *sqldb.Storage) {
	tb.Helper()
	db := target.Open(tb)
	table := livebackend.SQLTable(tb, db, "idemsql")
	opts = append(opts, sqldb.WithTableName(table), sqldb.WithTTL(ttl))
	s, err := sqldb.New(db, target.dialect, opts...)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), 3*time.Second)
		defer cancel()
		require.NoError(tb, s.Close(ctx))
	})
	require.NoError(tb, s.EnsureSchema(tb.Context()))
	require.NoError(tb, s.EnsureSchema(tb.Context()))
	return db, table, s
}

// Run includes the 256 KiB transport replay-record contract used by each
// storage's replay_record_test.go. Every contract gets its own table.
func TestSQLStorageContract(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, func(tb testing.TB) storages.Storage {
				_, _, s := target.open(tb, longTTL)
				return s
			})
		})
	}
}

func TestSQLTransaction(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, s := target.open(t, longTTL)
			businessTable := table + "_biz"
			_, err := db.ExecContext(t.Context(), "CREATE TABLE "+businessTable+" (id INTEGER PRIMARY KEY)")
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
				defer cancel()
				_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+businessTable)
				require.NoError(t, err)
			})
			for _, commit := range []bool{false, true} {
				tx, err := db.BeginTx(t.Context(), nil)
				require.NoError(t, err)
				t.Cleanup(func() { _ = tx.Rollback() })
				ctx := sqldb.WithTx(t.Context(), tx)
				// Rollback also removes the inserted lock and the business write.
				acquired, _, token, err := s.AttemptLock(ctx, "command", payload("lock"))
				require.NoError(t, err)
				require.True(t, acquired)
				_, err = tx.ExecContext(ctx, "INSERT INTO "+businessTable+" (id) VALUES (1)")
				require.NoError(t, err)
				require.NoError(t, s.Complete(ctx, "command", payload("done"), token))
				if commit {
					require.NoError(t, tx.Commit())
				} else {
					require.NoError(t, tx.Rollback())
				}
				var count int
				require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+businessTable).Scan(&count))
				if commit {
					require.Equal(t, 1, count)
					require.Equal(t, payload("done"), observe(t, s, "command"))
				} else {
					require.Zero(t, count)
					requireFree(t, s, "command")
				}
			}
		})
	}
}

func TestSQLExpiredRows(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, s := target.open(t, longTTL, sqldb.WithPurgeInterval(0), sqldb.WithPurgeBatchSize(1))
			token := lock(t, s, "expired", payload("old"))
			lock(t, s, "expired-too", payload("other"))
			lock(t, s, "live", payload("live"))
			// Force expiry with the backend clock without waiting or relying on the
			// TTL cleanup schedule. An expired row must reject all owner operations.
			expiry := "statement_timestamp() - INTERVAL '1 second'"
			if target.dialect == sqldb.DialectMySQL {
				expiry = "UTC_TIMESTAMP(6) - INTERVAL 1 SECOND"
			}
			keyColumn := `"key"`
			if target.dialect == sqldb.DialectMySQL {
				keyColumn = "`key`"
			}
			_, err := db.ExecContext(t.Context(), "UPDATE "+table+" SET expires_at = "+expiry+" WHERE "+keyColumn+" <> 'live'")
			require.NoError(t, err)
			require.ErrorIs(t, s.Complete(t.Context(), "expired", payload("late"), token), storages.ErrLockStolen)
			require.ErrorIs(t, s.Release(t.Context(), "expired", token), storages.ErrLockStolen)
			didSteal, _, _, err := s.Steal(t.Context(), "expired", payload("old"), payload("late"), 0, 0)
			require.NoError(t, err)
			require.False(t, didSteal)
			n, err := s.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Equal(t, int64(2), n)
			n, err = s.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, n)
			require.Equal(t, payload("live"), observe(t, s, "live"))
			requireFree(t, s, "expired")
		})
	}
}

func TestSQLKeysAndIdenticalWrites(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			_, _, s := target.open(t, longTTL)
			keys := []string{"Key", "key", "key ", strings.Repeat("ü", 255)}
			for _, key := range keys {
				token := lock(t, s, key, payload(key))
				require.NoError(t, s.Complete(t.Context(), key, payload(key), token), "unchanged bytes must still match")
			}
			for _, key := range keys {
				require.Equal(t, payload(key), observe(t, s, key), "keys must remain distinct")
				didSteal, current, _, err := s.Steal(t.Context(), key, payload(key), payload(key), 0, 0)
				require.NoError(t, err)
				require.False(t, didSteal, "completed values cannot be reclaimed")
				require.Equal(t, payload(key), current)
			}
			_, _, _, err := s.AttemptLock(t.Context(), strings.Repeat("ü", 256), nil)
			require.ErrorIs(t, err, sqldb.ErrValueTooLong)
		})
	}
}

func TestSQLFactory(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, _ := target.open(t, longTTL)
			_, err := db.ExecContext(t.Context(), "DROP TABLE "+table)
			require.NoError(t, err)
			cfg := idempotencyconfig.Default()
			cfg.MaxLockDuration = time.Minute
			cfg.Storage = &idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeSQL,
				SQL: &idempotencyconfig.SQLStorageConfig{Dialect: string(target.dialect), Table: table, EnsureSchema: true}}
			hooks := &coreruntime.HookGroup{}
			t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
			k, err := idemfactory.New(&cfg).UseSQLDB(db).UseShutdownHooks(hooks).Build(t.Context())
			require.NoError(t, err)
			ok, state, err := k.AttemptLock(t.Context(), "factory-key")
			require.NoError(t, err)
			require.True(t, ok)
			require.NoError(t, k.Complete(t.Context(), "factory-key", "result", state))
			ok, state, err = k.AttemptLock(t.Context(), "factory-key")
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, "result", state.Data)
		})
	}
}

func TestSQLIdenticalContenders(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, s := target.open(t, longTTL)
			for _, expired := range []bool{false, true} {
				if expired {
					expiry := "statement_timestamp() - INTERVAL '1 second'"
					if target.dialect == sqldb.DialectMySQL {
						expiry = "UTC_TIMESTAMP(6) - INTERVAL 1 SECOND"
					}
					_, err := db.ExecContext(t.Context(), "UPDATE "+table+" SET expires_at = "+expiry)
					require.NoError(t, err)
				}
				// Every contender writes the same binary bytes. Acquisition must depend
				// on winning the write, never on equality with the stored value.
				wins, err := concurrency.ProcessCollect(t.Context(), make([]int, contenders), func(ctx context.Context, _ int) (bool, error) {
					acquired, _, _, err := s.AttemptLock(ctx, "same-key", payload("identical"))
					return acquired, err
				}, concurrency.WithConcurrency[int](contenders))
				require.NoError(t, err)
				count := 0
				for _, won := range wins {
					if won {
						count++
					}
				}
				require.Equal(t, 1, count)
			}
		})
	}
}

func TestSQLTransactionReadsCurrentBytes(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, s := target.open(t, longTTL)
			lock(t, s, "key", payload("old"))
			isolation := sql.LevelReadCommitted
			if target.dialect == sqldb.DialectMySQL {
				isolation = sql.LevelRepeatableRead
			}
			tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: isolation})
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			// Establish a snapshot before another connection changes the value.
			var count int
			require.NoError(t, tx.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count))
			require.NoError(t, s.Complete(t.Context(), "key", payload("current"), nil))
			ctx := sqldb.WithTx(t.Context(), tx)
			acquired, existing, _, err := s.AttemptLock(ctx, "key", payload("next"))
			if target.name == "mariadb" {
				require.ErrorIs(t, err, sqldb.ErrRestartTransaction)
				return
			}
			require.NoError(t, err)
			require.False(t, acquired)
			require.Equal(t, payload("current"), existing)
		})
	}
}

func TestSQLBackgroundCleanup(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table, s := target.open(t, longTTL, sqldb.WithPurgeInterval(10*time.Millisecond))
			lock(t, s, "expired", payload("old"))
			lock(t, s, "live", payload("live"))
			expiry, keyColumn := "statement_timestamp() - INTERVAL '1 second'", `"key"`
			if target.dialect == sqldb.DialectMySQL {
				expiry, keyColumn = "UTC_TIMESTAMP(6) - INTERVAL 1 SECOND", "`key`"
			}
			_, err := db.ExecContext(t.Context(), "UPDATE "+table+" SET expires_at = "+expiry+" WHERE "+keyColumn+" = 'expired'")
			require.NoError(t, err)
			testhelpers.WaitFor(t, 3*time.Second, func() bool {
				var count int
				err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count)
				return err == nil && count == 1
			}, "background cleanup must physically remove expired rows")
			require.Equal(t, payload("live"), observe(t, s, "live"))
			require.NoError(t, s.Close(t.Context()))
		})
	}
}
