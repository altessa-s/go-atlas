//go:build volume

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"database/sql"
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
			// Formats: reliability/idempotency/storages/sqldb/schema.go:23-38.
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
			analyze := "ANALYZE " + quotedTable
			if target.dialect == sqldb.DialectMySQL {
				analyze = "ANALYZE TABLE " + quotedTable
			}
			_, err := db.ExecContext(t.Context(), analyze)
			require.NoError(t, err)
			var key any = "k00000001"
			if target.dialect == sqldb.DialectMySQL {
				key = []byte("k00000001")
			}
			// Query shapes: reliability/idempotency/storages/sqldb/store.go:70-85;
			// Clock/TTL expressions: reliability/idempotency/storages/sqldb/dialect.go:49-70.
			t.Run("explain_observe", func(t *testing.T) {
				volumeSQLExplain(t, db, style, table, "PRIMARY", "SELECT val FROM "+quotedTable+
					" WHERE "+keyColumn+" = ? AND expires_at > "+clock+" FOR UPDATE", key)
			})
			t.Run("explain_takeover", func(t *testing.T) {
				volumeSQLExplain(t, db, style, table, "PRIMARY", "UPDATE "+quotedTable+" SET val = ?, expires_at = "+expiry+
					", revision = revision + 1 WHERE "+keyColumn+" = ? AND expires_at <= "+clock, payload("takeover"), ttl, key)
			})
			t.Run("explain_cleanup", func(t *testing.T) {
				index := strings.Trim(style.IndexName(table, "expires_key"), "\"`")
				query := "SELECT " + keyColumn + " FROM " + quotedTable + " WHERE expires_at <= " + clock +
					" ORDER BY expires_at, " + keyColumn + " LIMIT ?"
				if target.dialect == sqldb.DialectPostgres {
					volumePostgresPurgeExplain(t, db, table, index, keyColumn, query, n, 1000)
					return
				}
				volumeSQLExplain(t, db, style, table, index, query, 1000)
			})

			volumeStorageContenders(t, storage, n)
			expected := (n+1)/2 - (min(2000, n/10)+1)/2
			var expired int64
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+quotedTable+
				" WHERE expires_at <= "+clock).Scan(&expired))
			require.EqualValues(t, expected, expired)
			livebackend.Measure(t, "purge SQL idempotency rows", expected, func() {
				deleted, err := storage.PurgeExpired(t.Context())
				require.NoError(t, err)
				require.Equal(t, expired, deleted)
				var remaining int64
				require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+quotedTable).Scan(&remaining))
				require.EqualValues(t, n-expected, remaining)
				deleted, err = storage.PurgeExpired(t.Context())
				require.NoError(t, err)
				require.Zero(t, deleted)
			})
		})
	}
}

func TestVolume_MongoStorage(t *testing.T) {
	db, storage := volumeMongoStorage(t)
	coll := db.Collection(idemmongo.DefaultCollectionName)
	n := livebackend.VolumeRows(t)
	now := time.Now().UTC()
	// Fields: reliability/idempotency/storages/mongo/mongo.go:23-31, :94-96.
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
	// The _id filter matches mongo.go:139; the expiry index is created at :83-85.
	t.Run("explain_key", func(t *testing.T) {
		volumeMongoExplain(t, coll, bson.M{"_id": "k00000001"}, "")
	})
	t.Run("explain_expiry", func(t *testing.T) {
		volumeMongoExplain(t, coll, bson.M{"expires_at": bson.M{"$lte": now}}, "expires_at_1")
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

// volumeSQLExplain checks the plan of query on the dialect's EXPLAIN helper;
// index is the MySQL index the query must use.
func volumeSQLExplain(tb testing.TB, db *sql.DB, style sqldialect.Style, table, index, query string, args ...any) {
	tb.Helper()
	if style.Numbered {
		tb.Logf("%s: indexes %v", query, livebackend.ExplainPostgres(tb, db, table, style.Bind(query, 1), args...))
		return
	}
	access, key := livebackend.ExplainMySQL(tb, db, table, []string{index}, query, args...)
	tb.Logf("%s: access %s, key %s", query, access, key)
}

// volumeMongoExplain checks that a find with filter uses an index, and index
// when it is not empty.
func volumeMongoExplain(tb testing.TB, coll *mongo.Collection, filter bson.M, index string) {
	tb.Helper()
	indexes := livebackend.ExplainMongo(tb, coll.Database(), bson.D{{Key: "find", Value: coll.Name()}, {Key: "filter", Value: filter}})
	tb.Logf("MongoDB filter %v: indexes %v", filter, indexes)
	if index != "" {
		require.Contains(tb, indexes, index)
	}
}

// volStrictPurgePlanRows is the size from which the PostgreSQL purge DELETE
// must not read the table sequentially, as in auditit.
const volStrictPurgePlanRows = 100_000

// volumePostgresPurgeExplain checks the PostgreSQL purge of the store: the
// batch select must use index at any size, and the DELETE joining it to the
// table must not read the table sequentially from volStrictPurgePlanRows on.
// Over a smaller table PostgreSQL joins the batch with a hash semi join that
// reads the table whole, cheaper there than one primary-key probe per row, so
// the plan is only logged.
func volumePostgresPurgeExplain(tb testing.TB, db *sql.DB, table, index, column, batch string, n int, args ...any) {
	tb.Helper()
	inner := batch + " FOR UPDATE SKIP LOCKED"
	indexes := livebackend.ExplainPostgres(tb, db, table, sqldialect.Postgres.Bind(inner, 1), args...)
	require.Contains(tb, indexes, index, "the purge batch select must use %s", index)
	del := sqldialect.Postgres.Bind("DELETE FROM "+sqldialect.Postgres.Ident(table)+" WHERE "+column+" IN ("+inner+")", 1)
	if n >= volStrictPurgePlanRows {
		tb.Logf("purge plan indexes: %v", livebackend.ExplainPostgres(tb, db, table, del, args...))
		return
	}
	var plan string
	require.NoError(tb, db.QueryRowContext(tb.Context(), "EXPLAIN (FORMAT JSON) "+del, args...).Scan(&plan))
	tb.Logf("purge plan below %d rows, not checked: %s", volStrictPurgePlanRows, strings.Join(strings.Fields(plan), " "))
}
