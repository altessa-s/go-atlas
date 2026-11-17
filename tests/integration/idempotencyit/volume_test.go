//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/internal/sqldialect"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	idemmongo "github.com/altessa-s/go-atlas/reliability/idempotency/storages/mongo"
)

func TestVolume_SQLStorage(t *testing.T) {
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			db, table, storage := target.open(t, longTTL, sqldb.WithPurgeInterval(0))
			// Keep the contenders' connections open: churn exhausts ephemeral ports.
			db.SetMaxIdleConns(32)
			n := livebackend.VolumeRows(t)
			now := time.Now().UTC()
			style := sqldialect.Postgres
			clock, expiry, ttl := "statement_timestamp()", "(statement_timestamp() + make_interval(secs => ?))", any(longTTL.Seconds())
			if target.dialect == sqldb.DialectMySQL {
				style = sqldialect.MySQL
				clock, expiry, ttl = "UTC_TIMESTAMP(6)", "(UTC_TIMESTAMP(6) + INTERVAL ? MICROSECOND)", longTTL.Microseconds()
			}
			keyColumn := style.Ident("key")
			quotedTable := style.Ident(table)
			// Formats: reliability/idempotency/storages/sqldb EnsureSchema.
			livebackend.Measure(t, "seed SQL idempotency rows", n, func() {
				livebackend.SQLBulkInsert(t, db, string(target.dialect), quotedTable,
					[]string{keyColumn, "val", "expires_at", "revision"}, n, func(i int) []any {
						expires := now.Add(time.Hour)
						if i%2 == 0 {
							expires = now.Add(-time.Hour)
						}
						key := fmt.Sprintf("k%08d", i)
						var keyArg, expiresArg any = key, expires
						if target.dialect == sqldb.DialectMySQL {
							keyArg = []byte(key)
							// The fixture DSN need not enable parseTime or set loc.
							expiresArg = expires.Format("2006-01-02 15:04:05.999999")
						}
						return []any{keyArg, payload("seed"), expiresArg, int64(0)}
					})
			})
			livebackend.Analyze(t, db, string(target.dialect), quotedTable)
			var key any = "k00000001"
			if target.dialect == sqldb.DialectMySQL {
				key = []byte("k00000001")
			}
			// Query shapes: reliability/idempotency/storages/sqldb buildStatements;
			// Clock/TTL expressions: reliability/idempotency/storages/sqldb dialectFor, expiresIn and duration.
			primary := "PRIMARY"
			if style.Numbered {
				primary = table + "_pkey"
			}
			t.Run("explain_observe", func(t *testing.T) {
				livebackend.ExplainSQL(t, db, style, table, primary, "SELECT val FROM "+quotedTable+
					" WHERE "+keyColumn+" = ? AND expires_at > "+clock+" FOR UPDATE", key)
			})
			t.Run("explain_takeover", func(t *testing.T) {
				livebackend.ExplainSQL(t, db, style, table, primary, "UPDATE "+quotedTable+" SET val = ?, expires_at = "+expiry+
					", revision = revision + 1 WHERE "+keyColumn+" = ? AND expires_at <= "+clock, payload("takeover"), ttl, key)
			})
			t.Run("explain_cleanup", func(t *testing.T) {
				index := strings.Trim(style.IndexName(table, "expires_key"), "\"`")
				query := "SELECT " + keyColumn + " FROM " + quotedTable + " WHERE expires_at <= " + clock +
					" ORDER BY expires_at, " + keyColumn + " LIMIT ?"
				if target.dialect == sqldb.DialectPostgres {
					livebackend.ExplainPostgresPurge(t, db, table, index, keyColumn, query, n, 1000)
					return
				}
				livebackend.ExplainSQL(t, db, style, table, index, query, 1000)
			})

			volumeStorageContenders(t, storage, n)
			expected := (n+1)/2 - (min(2000, n/10)+1)/2
			var expired int64
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+quotedTable+
				" WHERE expires_at <= "+clock).Scan(&expired))
			require.EqualValues(t, expected, expired)
			var deleted int64
			livebackend.Measure(t, "purge SQL idempotency rows", expected, func() {
				var err error
				deleted, err = storage.PurgeExpired(t.Context())
				require.NoError(t, err)
			})
			require.Equal(t, expired, deleted)
			var remaining int64
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+quotedTable).Scan(&remaining))
			require.EqualValues(t, n-expected, remaining)
			deleted, err := storage.PurgeExpired(t.Context())
			require.NoError(t, err)
			require.Zero(t, deleted)
		})
	}
}

func TestVolume_MongoStorage(t *testing.T) {
	db, storage := volumeMongoStorage(t)
	coll := db.Collection(idemmongo.DefaultCollectionName)
	n := livebackend.VolumeRows(t)
	now := time.Now().UTC()
	// Fields: reliability/idempotency/storages/mongo field constants and document.
	// A seed needs no attempt marker: live rows keep their bytes and expired
	// rows receive a fresh marker when AttemptLock takes them over (:125-134).
	livebackend.Measure(t, "seed MongoDB idempotency documents", n, func() {
		livebackend.MongoBulkInsert(t, coll, n, func(i int) any {
			expires := now.Add(time.Hour)
			if i%2 == 0 {
				expires = now.Add(-time.Hour)
			}
			return bson.M{"_id": fmt.Sprintf("k%08d", i), "val": payload("seed"), "expires_at": expires}
		})
	})
	// The _id filter matches AttemptLockWithTTL; the expiry index is created in newStorage.
	t.Run("explain_key", func(t *testing.T) {
		livebackend.ExplainMongoFind(t, coll, bson.M{"_id": "k00000001"}, "")
	})
	t.Run("explain_expiry", func(t *testing.T) {
		livebackend.ExplainMongoFind(t, coll, bson.M{"expires_at": bson.M{"$lte": now}}, "expires_at_1")
	})
	// The TTL monitor may remove expired seeds; missing and expired keys are
	// both free. MongoDB has no PurgeExpired: its TTL index performs cleanup.
	volumeStorageContenders(t, storage, n)
}

func volumeMongoStorage(tb testing.TB) (*mongo.Database, *idemmongo.Storage) {
	tb.Helper()
	db := livebackend.MongoDatabase(tb, "idemvolume")
	storage, err := idemmongo.New(db, idemmongo.WithTTL(longTTL))
	require.NoError(tb, err)
	return db, storage
}

func volumeStorageContenders(tb testing.TB, storage storages.Storage, n int) {
	tb.Helper()
	const contenders = 8
	k := min(2000, n/10)
	livebackend.Measure(tb, "contend and complete idempotency keys", k*contenders, func() {
		for i := range k {
			key := fmt.Sprintf("k%08d", i)
			var results [contenders]struct {
				acquired        bool
				existing, token []byte
			}
			err := concurrency.Process(tb.Context(), []int{0, 1, 2, 3, 4, 5, 6, 7}, func(ctx context.Context, c int) error {
				var err error
				results[c].acquired, results[c].existing, results[c].token, err = storage.AttemptLock(ctx, key,
					payload(fmt.Sprintf("contender-%d", c)))
				return err
			}, concurrency.WithConcurrency[int](contenders))
			require.NoError(tb, err, "key %s", key)
			wins := 0
			var token []byte
			for c, result := range results {
				if result.acquired {
					wins++
					token = result.token
					require.Equal(tb, payload(fmt.Sprintf("contender-%d", c)), token, "key %s", key)
				}
			}
			stored := payload("seed")
			if i%2 == 0 {
				require.Equal(tb, 1, wins, "expired key %s", key)
				stored = token
			} else {
				require.Zero(tb, wins, "live key %s", key)
			}
			for _, result := range results {
				if !result.acquired {
					require.Equal(tb, stored, result.existing, "loser must observe stored bytes for %s", key)
					require.Empty(tb, result.token)
				}
			}
			if wins == 1 {
				require.NoError(tb, storage.Complete(tb.Context(), key, payload("complete"), token), "key %s", key)
				require.Equal(tb, payload("complete"), observe(tb, storage, key))
			}
		}
	})
}
