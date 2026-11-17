//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	lockmongo "github.com/altessa-s/go-atlas/coord/dlock/providers/mongo"
)

func TestVolume_SQLLocker(t *testing.T) {
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			db, table := target.namespace(t)
			// Keep the contenders' connections open: churn exhausts ephemeral ports.
			db.SetMaxIdleConns(32)
			locker := target.provider(t, db, table, sqldb.WithExpiredCleanup(0), sqldb.WithPurgeInterval(0))
			require.NoError(t, locker.EnsureSchema(t.Context()))
			n := livebackend.VolumeRows(t)
			nowUs := time.Now().UTC().UnixMicro()
			const ttlUs = int64(time.Hour / time.Microsecond)
			style := sqldialect.Postgres
			clock := "CAST(EXTRACT(EPOCH FROM clock_timestamp()) * 1000000 AS BIGINT)"
			if target.dialect == sqldb.DialectMySQL {
				style = sqldialect.MySQL
				clock = "TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', UTC_TIMESTAMP(6))"
			}
			quotedTable := style.Ident(table)
			// Formats: coord/dlock/providers/sqldb postgresSchema and mysqlSchema. All times
			// are Unix microseconds; binary MySQL keys match dialect.textArg.
			livebackend.Measure(t, "seed SQL lock rows", n, func() {
				livebackend.SQLBulkInsert(t, db, string(target.dialect), quotedTable,
					[]string{"lock_key", "owner", "fencing", "acquired_at", "renewed_at", "expires_at", "ttl_us"},
					n, func(i int) []any {
						expires, owner := nowUs-ttlUs, ""
						if i%10 == 9 {
							expires, owner = nowUs+ttlUs, "seed"
						}
						key := fmt.Sprintf("k%08d", i)
						var keyArg, ownerArg any = key, owner
						if target.dialect == sqldb.DialectMySQL {
							keyArg, ownerArg = []byte(key), []byte(owner)
						}
						return []any{keyArg, ownerArg, int64(1), expires - ttlUs, expires - ttlUs, expires, ttlUs}
					})
			})
			require.Equal(t, n, rowCount(t, db, table))
			livebackend.Analyze(t, db, string(target.dialect), quotedTable)
			var key, owner any = "k00000000", "candidate"
			primary := table + "_pkey"
			if target.dialect == sqldb.DialectMySQL {
				key, owner, primary = []byte("k00000000"), []byte("candidate"), "PRIMARY"
			}
			// Query shapes: coord/dlock/providers/sqldb store.buildStatements;
			// Database clocks: coord/dlock/providers/sqldb dialectFor.
			t.Run("explain_acquire", func(t *testing.T) {
				query := "INSERT INTO " + quotedTable + " AS cur (lock_key, owner, fencing, acquired_at, renewed_at, expires_at, ttl_us)" +
					" VALUES (?, ?, 1, " + clock + ", " + clock + ", " + clock + " + ?, ?)" +
					" ON CONFLICT (lock_key) DO UPDATE SET owner = EXCLUDED.owner, fencing = cur.fencing + 1," +
					" acquired_at = " + clock + ", renewed_at = " + clock + ", expires_at = " + clock +
					" + EXCLUDED.ttl_us, ttl_us = EXCLUDED.ttl_us WHERE cur.expires_at <= " + clock + " RETURNING fencing"
				args := []any{key, owner, ttlUs, ttlUs}
				if target.dialect == sqldb.DialectMySQL {
					query = "UPDATE " + quotedTable + " SET owner = ?, fencing = fencing + 1, acquired_at = " + clock +
						", renewed_at = " + clock + ", expires_at = " + clock + " + ?, ttl_us = ? WHERE lock_key = ? AND expires_at <= " + clock
					args = []any{owner, ttlUs, ttlUs, key}
				}
				index := primary
				if target.dialect == sqldb.DialectPostgres {
					index = ""
				}
				livebackend.ExplainSQL(t, db, style, table, index, query, args...)
			})
			t.Run("explain_renew", func(t *testing.T) {
				livebackend.ExplainSQL(t, db, style, table, primary, "UPDATE "+quotedTable+" SET renewed_at = "+clock+
					", expires_at = "+clock+" + ? WHERE lock_key = ? AND owner = ? AND fencing = ? AND expires_at > "+clock,
					ttlUs, key, owner, int64(1))
			})
			t.Run("explain_renew_lock_row", func(t *testing.T) {
				livebackend.ExplainSQL(t, db, style, table, primary, "SELECT 1 FROM "+quotedTable+" WHERE lock_key = ? FOR UPDATE", key)
			})
			index := strings.Trim(style.IndexName(table, "expires"), "\"`")
			// The cleanup cutoff reads the clock once per statement (dialect.go cutoffNow).
			cutoff := strings.ReplaceAll(clock, "clock_timestamp()", "statement_timestamp()")
			cleanup := "SELECT lock_key FROM " + quotedTable + " WHERE expires_at <= " + cutoff + " - ? ORDER BY expires_at LIMIT ?"
			t.Run("explain_cleanup", func(t *testing.T) {
				if target.dialect == sqldb.DialectPostgres {
					livebackend.ExplainPostgresPurge(t, db, table, index, "lock_key", cleanup, n, int64(0), 1000)
					return
				}
				livebackend.ExplainSQL(t, db, style, table, index, cleanup, int64(0), 1000)
			})

			volumeLockerContenders(t, locker, n)
			// Release writes the database's current time. Count by the same
			// database clock the cleanup cutoff reads, after every release, so
			// those rows count whatever the host clock says.
			var expected int64
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+quotedTable+
				" WHERE expires_at <= "+cutoff).Scan(&expected))
			var deleted int64
			livebackend.Measure(t, "purge SQL lock rows", int(expected), func() {
				var err error
				deleted, err = locker.PurgeExpired(t.Context())
				require.NoError(t, err)
			})
			require.Equal(t, expected, deleted)
			require.Equal(t, n-int(expected), rowCount(t, db, table))
			deleted, err := locker.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, deleted)
		})
	}
}

func TestVolume_MongoLocker(t *testing.T) {
	db := livebackend.MongoDatabase(t, "dlockvol")
	locker := mongoLocker(t, db, lockmongo.WithExpiredCleanup(0))
	require.NoError(t, locker.EnsureIndexes(t.Context()))
	coll := db.Collection(lockmongo.DefaultCollection)
	n := livebackend.VolumeRows(t)
	now := time.Now().UTC()
	// Exact lockDoc fields: coord/dlock/providers/mongo lockDoc.
	livebackend.Measure(t, "seed MongoDB lock documents", n, func() {
		livebackend.MongoBulkInsert(t, coll, n, func(i int) any {
			expires, owner := now.Add(-time.Hour), ""
			if i%10 == 9 {
				expires, owner = now.Add(time.Hour), "seed"
			}
			return bson.M{
				"_id": fmt.Sprintf("k%08d", i), "owner": owner, "fencing": int64(1),
				"acquired_at": expires.Add(-time.Hour), "renewed_at": expires.Add(-time.Hour),
				"expires_at": expires, "ttl_ms": time.Hour.Milliseconds(),
			}
		})
	})
	// The acquisition filter is Acquire's _id match; the TTL index is the cleanup index.
	t.Run("explain_key", func(t *testing.T) {
		livebackend.ExplainMongoFind(t, coll, bson.M{"_id": "k00000000"}, "")
	})
	t.Run("explain_expiry", func(t *testing.T) {
		livebackend.ExplainMongoFind(t, coll, bson.M{"expires_at": bson.M{"$lte": now}}, "expires_at_ttl")
	})
	// Missing seeds and expired seeds are equally free: the TTL monitor may
	// remove them at any time. MongoDB has no exact purge API to count.
	volumeLockerContenders(t, locker, n)
}

func volumeLockerContenders(tb testing.TB, locker providers.Provider, n int) {
	tb.Helper()
	expired, live := min(500, n/10), min(50, n/10)
	livebackend.Measure(tb, "contend and release lock keys", (expired+live)*8, func() {
		for i := range expired {
			// Nine of each ten seed keys are expired.
			volumeLockerKey(tb, locker, fmt.Sprintf("k%08d", (i/9)*10+i%9), true)
		}
		for i := range live {
			volumeLockerKey(tb, locker, fmt.Sprintf("k%08d", i*10+9), false)
		}
	})
}

func volumeLockerKey(tb testing.TB, locker providers.Provider, key string, expired bool) {
	tb.Helper()
	const contenders = 8
	// All contenders share a short context that stays alive through the
	// assertions and releases. Canceling a winner early would free its lease
	// and let a later contender win sequentially.
	ctx, cancel := context.WithTimeout(tb.Context(), 10*time.Second)
	defer cancel()
	var results [contenders]struct {
		lock providers.Lock
		err  error
	}
	err := concurrency.Process(ctx, []int{0, 1, 2, 3, 4, 5, 6, 7}, func(ctx context.Context, c int) error {
		results[c].lock, results[c].err = locker.Lock(ctx, key)
		return nil
	}, concurrency.WithConcurrency[int](contenders))
	// Keep every successful lease held until all eight calls have returned.
	// Deferred releases also run if any assertion below fails.
	for _, result := range results {
		if result.lock != nil {
			defer func() { require.NoError(tb, result.lock.Release(context.WithoutCancel(tb.Context())), "key %s", key) }()
		}
	}
	require.NoError(tb, err, "key %s", key)
	wins := 0
	for _, result := range results {
		if result.err != nil {
			require.ErrorIs(tb, result.err, providers.ErrLockNotHeld, "key %s", key)
			require.Nil(tb, result.lock)
			continue
		}
		wins++
		require.NotNil(tb, result.lock, "key %s", key)
		info, err := result.lock.GetLockInfo(ctx)
		require.NoError(tb, err, "winner must still own %s after contention", key)
		require.Equal(tb, key, info.Key)
		require.NotEmpty(tb, info.Owner)
	}
	if expired {
		require.Equal(tb, 1, wins, "expired key %s must have exactly one holder", key)
	} else {
		require.Zero(tb, wins, "live seed key %s must stay held", key)
		info, err := locker.GetLockInfo(ctx, key)
		require.NoError(tb, err)
		require.Equal(tb, "seed", info.Owner)
	}
}
