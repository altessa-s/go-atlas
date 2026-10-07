// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/sqldb"
	"github.com/altessa-s/go-atlas/tests/integration/dlockit"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// sqlTarget is one database/sql server of the docker stack.
type sqlTarget struct {
	name, driver, envKey, dsn string
	dialect                   sqldb.Dialect
}

func sqlTargets() []sqlTarget {
	return []sqlTarget{
		{"postgres", "pgx", "POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable", sqldb.DialectPostgres},
		{"mariadb", "mysql", "MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas", sqldb.DialectMySQL},
		{"mysql", "mysql", "MYSQL_DSN", "atlas:atlas@tcp(127.0.0.1:13307)/atlas", sqldb.DialectMySQL},
	}
}

// namespace opens the server and creates a throwaway locks table, dropped on
// cleanup. It skips when the server is unreachable.
func (s sqlTarget) namespace(tb testing.TB) (*sql.DB, string) {
	tb.Helper()
	dsn := s.dsn
	if v := os.Getenv(s.envKey); v != "" {
		dsn = v
	}
	db, err := sql.Open(s.driver, dsn)
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		// The DSN is not printed: an override may carry a password.
		tb.Skipf("%s unreachable (%v) — start it with: make integration-up", s.name, err)
	}
	table := "dlock_" + strings.ToLower(rand.Text()[:12])
	tb.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		_ = db.Close()
	})
	l, err := sqldb.New(db, s.dialect, sqldb.WithTableName(table))
	require.NoError(tb, err)
	require.NoError(tb, l.EnsureSchema(tb.Context()))
	require.NoError(tb, l.EnsureSchema(tb.Context()), "EnsureSchema is idempotent")
	return db, table
}

// provider returns a locker over table with a one-second TTL, closed on
// cleanup.
func (s sqlTarget) provider(tb testing.TB, db *sql.DB, table string) *sqldb.Locker {
	tb.Helper()
	l, err := sqldb.New(db, s.dialect, sqldb.WithTableName(table), sqldb.WithTTL(time.Second))
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = l.Close(context.Background()) })
	return l
}

// TestSQLProviderContract runs the provider contract suite on every SQL
// server, each contract over its own table. Expire ages a lease out by
// writing expires_at directly, as a holder that stopped renewing would leave
// it.
func TestSQLProviderContract(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			providertest.Run(t, func(tb testing.TB) providertest.Backend {
				db, table := target.namespace(tb)
				return providertest.Backend{
					NewProvider: func(tb testing.TB) providers.Provider { return target.provider(tb, db, table) },
					Expire: func(tb testing.TB, key string) {
						query := "UPDATE " + table + " SET expires_at = 0 WHERE lock_key = ?"
						if target.dialect == sqldb.DialectPostgres {
							query = strings.Replace(query, "?", "$1", 1)
						}
						_, err := db.ExecContext(tb.Context(), query, key)
						require.NoError(tb, err)
					},
				}
			})
		})
	}
}

// TestSQLContendersNeverOverlap races several dlock instances, each over its
// own SQL provider, for one key and records every critical section.
func TestSQLContendersNeverOverlap(t *testing.T) {
	t.Parallel()
	const contenders, rounds = 6, 3
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.namespace(t)
			var (
				section dlockit.Critical
				wg      sync.WaitGroup
			)
			for i := range contenders {
				dl := dlock.New(target.provider(t, db, table))
				holder := fmt.Sprintf("holder-%d", i)
				wg.Go(func() {
					for range rounds {
						synchronizeUntilDone(t, dl, "resource", func(context.Context) error {
							leave := section.Enter(holder)
							defer leave()
							time.Sleep(holdTime)
							return nil
						})
					}
				})
			}
			wg.Wait()
			require.Equal(t, 1, section.Peak(), "more than one holder was inside the critical section\n%s", section.Timeline())
			require.Empty(t, section.Breaches(), "critical sections overlapped\n%s", section.Timeline())
			require.Len(t, section.Visits(), contenders*rounds)
		})
	}
}

// TestSQLRenewalAfterLockWaitDoesNotRevive holds the lock row in another
// transaction past the lease's TTL, so the holder's renewal waits for it. When
// the row is released the lease has expired, and the renewal must find it
// lost rather than extend it with the clock it read before waiting.
func TestSQLRenewalAfterLockWaitDoesNotRevive(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.namespace(t)
			l := target.provider(t, db, table)
			_, err := l.Lock(t.Context(), "k")
			require.NoError(t, err)

			tx, err := db.BeginTx(t.Context(), nil)
			require.NoError(t, err)
			query := "SELECT 1 FROM " + table + " WHERE lock_key = ? FOR UPDATE"
			if target.dialect == sqldb.DialectPostgres {
				query = strings.Replace(query, "?", "$1", 1)
			}
			rows, err := tx.QueryContext(t.Context(), query, "k")
			require.NoError(t, err)
			require.NoError(t, rows.Close())

			// The lease, taken at 0 with a one-second TTL, expires at 1s; the
			// first renewal starts at about 333ms and waits. Releasing the row
			// at 1.15s leaves a stale-clock renewal enough room to revive the
			// lease until about 1.33s, which the check below would see.
			time.Sleep(1150 * time.Millisecond)
			require.NoError(t, tx.Commit())
			time.Sleep(50 * time.Millisecond)

			_, err = l.GetLockInfo(t.Context(), "k")
			require.Error(t, err, "the expired lease must stay lost, not be revived by the waiting renewal")
		})
	}
}
