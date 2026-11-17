// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
	"github.com/altessa-s/go-atlas/tests/integration/dlockit"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
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

// namespace opens the server and creates a throwaway locks table, dropped on
// cleanup. It skips when the server is unreachable.
func (s sqlTarget) namespace(tb testing.TB) (*sql.DB, string) {
	tb.Helper()
	db := s.Open(tb)
	table := livebackend.SQLTable(tb, db, "dlock")
	l, err := sqldb.New(db, s.dialect, sqldb.WithTableName(table))
	require.NoError(tb, err)
	require.NoError(tb, l.EnsureSchema(tb.Context()))
	require.NoError(tb, l.EnsureSchema(tb.Context()), "EnsureSchema is idempotent")
	return db, table
}

// provider returns a locker over table with a fifteen-second TTL, closed on
// cleanup.
func (s sqlTarget) provider(tb testing.TB, db *sql.DB, table string, opts ...sqldb.Option) *sqldb.Locker {
	tb.Helper()
	const ttl = 15 * time.Second
	l, err := sqldb.New(db, s.dialect, append([]sqldb.Option{sqldb.WithTableName(table), sqldb.WithTTL(ttl)}, opts...)...)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = l.Close(context.WithoutCancel(tb.Context())) })
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
				var opts []sqldb.Option
				if strings.HasSuffix(tb.Name(), "/Renewal") || strings.HasSuffix(tb.Name(), "/ExpiredLeaseStaysLost") {
					opts = append(opts, sqldb.WithTTL(time.Second))
				}
				return providertest.Backend{
					NewProvider: func(tb testing.TB) providers.Provider { return target.provider(tb, db, table, opts...) },
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
			l := target.provider(t, db, table, sqldb.WithTTL(time.Second))
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

// bind rewrites ? placeholders to $n for PostgreSQL.
func (s sqlTarget) bind(query string) string {
	if s.dialect != sqldb.DialectPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// rowCount returns the number of rows in table.
func rowCount(tb testing.TB, db *sql.DB, table string) int {
	tb.Helper()
	var n int
	require.NoError(tb, db.QueryRowContext(tb.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

// TestSQLHardLeaseEndsWhileHeld asserts a lease taken WithoutRenewal is not
// extended: another locker takes the key one TTL later although the first
// holder never released it, and the first holder then sees its lock lost.
func TestSQLHardLeaseEndsWhileHeld(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.namespace(t)
			const ttl = time.Second
			a := target.provider(t, db, table, sqldb.WithoutRenewal(), sqldb.WithTTL(ttl))
			b := target.provider(t, db, table, sqldb.WithoutRenewal(), sqldb.WithTTL(ttl))

			held, err := a.Lock(t.Context(), "k")
			require.NoError(t, err)
			first, err := held.GetLockInfo(t.Context())
			require.NoError(t, err)
			_, err = b.Lock(t.Context(), "k")
			require.ErrorIs(t, err, providers.ErrLockNotHeld)

			var taken providers.Lock
			require.Eventually(t, func() bool {
				taken, err = b.Lock(t.Context(), "k")
				return err == nil
			}, 5*ttl, 50*time.Millisecond, "the hard lease must end one TTL after it was taken")
			t.Cleanup(func() { _ = taken.Release(context.WithoutCancel(t.Context())) })

			info, err := taken.GetLockInfo(t.Context())
			require.NoError(t, err)
			require.Equal(t, first.FencingToken+1, info.FencingToken)
			// PostgreSQL reads clock_timestamp() per column, so the two may
			// differ by microseconds; a renewal would move LastRenewed by a
			// third of the TTL or more.
			require.WithinDuration(t, info.AcquiredAt, info.LastRenewed, 10*time.Millisecond, "a hard lease is never renewed")
			_, err = held.GetLockInfo(t.Context())
			require.ErrorIs(t, err, providers.ErrLockNotHeld, "the first holder's lease has ended")
			require.NoError(t, held.Release(t.Context()))
			_, err = taken.GetLockInfo(t.Context())
			require.NoError(t, err, "the ended holder's release must not end the new lease")
		})
	}
}

// TestSQLExpiredCleanup asserts PurgeExpired keeps rows within the grace and
// held leases, deletes rows whose lease ended longer ago — in batches, however
// many there are — and that a deleted key starts again at fencing token 1.
func TestSQLExpiredCleanup(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.namespace(t)

			off := target.provider(t, db, table)
			n, err := off.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, n, "without WithExpiredCleanup nothing is deleted")

			patient := target.provider(t, db, table, sqldb.WithExpiredCleanup(time.Hour))
			eager := target.provider(t, db, table, sqldb.WithExpiredCleanup(0))

			held, err := patient.Lock(t.Context(), "held")
			require.NoError(t, err)
			t.Cleanup(func() { _ = held.Release(context.WithoutCancel(t.Context())) })
			for range 2 {
				lk, err := patient.Lock(t.Context(), "released")
				require.NoError(t, err)
				require.NoError(t, lk.Release(t.Context()))
			}

			n, err = patient.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, n, "a lease that ended within the grace is kept")
			require.Equal(t, 2, rowCount(t, db, table))

			// Stale rows from long ago, more than one cleanup batch of them.
			const stale = 1200
			const chunk = 400
			for start := 0; start < stale; start += chunk {
				args := make([]any, 0, chunk)
				values := make([]string, 0, chunk)
				for i := start; i < start+chunk; i++ {
					values = append(values, "(?, 'gone', 3, 1, 1, 1, 1)")
					args = append(args, fmt.Sprintf("stale-%04d", i))
				}
				_, err := db.ExecContext(t.Context(), target.bind("INSERT INTO "+table+
					" (lock_key, owner, fencing, acquired_at, renewed_at, expires_at, ttl_us) VALUES "+strings.Join(values, ", ")), args...)
				require.NoError(t, err)
			}
			n, err = patient.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, stale, n, "rows past the grace are deleted across batches")
			require.Equal(t, 2, rowCount(t, db, table))

			n, err = eager.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, n, "with no grace the released row goes, the held one stays")
			info, err := held.GetLockInfo(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, info.FencingToken)

			lk, err := eager.Lock(t.Context(), "released")
			require.NoError(t, err)
			info, err = lk.GetLockInfo(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, info.FencingToken, "fencing is monotonic only within the grace")
			require.NoError(t, lk.Release(t.Context()))
		})
	}
}

// TestSQLCleanupRacesAcquisitions runs a cleanup with no grace in a loop while
// contenders take and release one key, so rows are deleted between and during
// acquisitions; mutual exclusion must hold throughout.
func TestSQLCleanupRacesAcquisitions(t *testing.T) {
	t.Parallel()
	const contenders, rounds = 4, 8
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.namespace(t)
			sweeper := target.provider(t, db, table, sqldb.WithExpiredCleanup(0))
			sweepCtx, stopSweep := context.WithCancel(t.Context())
			var (
				section dlockit.Critical
				wg      sync.WaitGroup
				sweeps  sync.WaitGroup
				deleted int64
			)
			sweeps.Go(func() {
				for sweepCtx.Err() == nil {
					n, err := sweeper.PurgeExpired(sweepCtx)
					if sweepCtx.Err() == nil && !assert.NoError(t, err) {
						return
					}
					deleted += n
				}
			})
			for i := range contenders {
				l := target.provider(t, db, table, sqldb.WithExpiredCleanup(0))
				holder := fmt.Sprintf("holder-%d", i)
				wg.Go(func() {
					for range rounds {
						lk, err := l.Lock(t.Context(), "resource")
						for deadline := time.Now().Add(30 * time.Second); errors.Is(err, providers.ErrLockNotHeld) && time.Now().Before(deadline); {
							time.Sleep(2 * time.Millisecond)
							lk, err = l.Lock(t.Context(), "resource")
						}
						// assert, not require: FailNow must run on the test goroutine.
						if !assert.NoError(t, err) {
							return
						}
						leave := section.Enter(holder)
						time.Sleep(10 * time.Millisecond)
						leave()
						if !assert.NoError(t, lk.Release(t.Context())) {
							return
						}
					}
				})
			}
			wg.Wait()
			stopSweep()
			sweeps.Wait()
			require.Equal(t, 1, section.Peak(), "more than one holder was inside the critical section\n%s", section.Timeline())
			require.Empty(t, section.Breaches(), "critical sections overlapped\n%s", section.Timeline())
			require.Len(t, section.Visits(), contenders*rounds)
			require.Positive(t, deleted, "the sweep deleted released rows")
		})
	}
}
