// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	mongostorage "github.com/altessa-s/go-atlas/data/idempotency/storages/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoURI returns the MongoDB connection string, honoring MONGO_URI.
func mongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

var sequence atomic.Uint64

// testDB connects to a live MongoDB, skipping the test when none is
// reachable, and returns a throwaway database dropped on cleanup.
func testDB(tb testing.TB) *mongo.Database {
	tb.Helper()
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()).SetTimeout(10 * time.Second))
	if err != nil {
		tb.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(tb.Context(), nil); err != nil {
		_ = client.Disconnect(context.WithoutCancel(tb.Context()))
		tb.Skipf("mongodb not reachable at %s: %v", mongoURI(), err)
	}
	db := client.Database("idempotency_it_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_" + strconv.FormatUint(sequence.Add(1), 10))
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), 5*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return db
}

// requireReplicaSet skips a test that needs transactions on a standalone
// server.
func requireReplicaSet(tb testing.TB, db *mongo.Database) {
	tb.Helper()
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	require.NoError(tb, db.RunCommand(tb.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello))
	if hello.SetName == "" && hello.Msg != "isdbgrid" {
		tb.Skip("transactions need a replica set or a sharded cluster")
	}
}

func newStorage(t *testing.T, opts ...mongostorage.Option) (*mongostorage.Storage, *mongo.Collection) {
	t.Helper()
	db := testDB(t)
	s, err := mongostorage.New(db, opts...)
	require.NoError(t, err)
	return s, db.Collection(mongostorage.DefaultCollectionName)
}

func TestStorageLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, _ := newStorage(t)

	ok, existing, token, err := s.AttemptLock(ctx, "k", []byte("lock-1"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Nil(t, existing)
	require.Equal(t, []byte("lock-1"), token)

	ok, existing, token, err = s.AttemptLock(ctx, "k", []byte("lock-2"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, []byte("lock-1"), existing)
	require.Nil(t, token)

	require.ErrorIs(t, s.Complete(ctx, "k", []byte("done"), []byte("lock-2")), storages.ErrLockStolen)
	require.NoError(t, s.Complete(ctx, "k", []byte("done"), []byte("lock-1")))
	_, existing, _, err = s.AttemptLock(ctx, "k", []byte("lock-3"))
	require.NoError(t, err)
	require.Equal(t, []byte("done"), existing)

	require.ErrorIs(t, s.Release(ctx, "k", []byte("lock-1")), storages.ErrLockStolen, "completed, no longer the lock")
	require.ErrorIs(t, s.Release(ctx, "k", nil), storages.ErrMissingLockState)
	require.NoError(t, s.Delete(ctx, "k"))
	ok, _, _, err = s.AttemptLock(ctx, "k", []byte("lock-4"))
	require.NoError(t, err)
	require.True(t, ok)

	newToken, err := s.Steal(ctx, "k", []byte("lock-4"), []byte("lock-5"))
	require.NoError(t, err)
	require.Equal(t, []byte("lock-5"), newToken)
	_, err = s.Steal(ctx, "k", []byte("lock-4"), []byte("lock-6"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
	require.NoError(t, s.Release(ctx, "k", []byte("lock-5")))

	for _, err := range []error{
		func() error { _, _, _, err := s.AttemptLock(ctx, "", nil); return err }(),
		s.Complete(ctx, "", nil, nil),
		s.Release(ctx, "", []byte("x")),
		s.Delete(ctx, ""),
	} {
		require.ErrorIs(t, err, storages.ErrEmptyKey)
	}
}

func TestStorageTreatsExpiredAsAbsent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, coll := newStorage(t)

	ok, _, _, err := s.AttemptLock(ctx, "k", []byte("old"))
	require.NoError(t, err)
	require.True(t, ok)
	// Expire it without waiting for the TTL monitor.
	_, err = coll.UpdateByID(ctx, "k", bson.D{{Key: "$set", Value: bson.D{{Key: "expires_at", Value: time.Now().Add(-time.Hour)}}}})
	require.NoError(t, err)

	require.ErrorIs(t, s.Complete(ctx, "k", []byte("done"), []byte("old")), storages.ErrLockStolen, "an expired lock is lost")
	ok, _, token, err := s.AttemptLock(ctx, "k", []byte("new"))
	require.NoError(t, err)
	require.True(t, ok, "an expired key is free")
	require.Equal(t, []byte("new"), token)
}

func TestStorageLockTTL(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, coll := newStorage(t, mongostorage.WithTTL(time.Hour))

	_, _, _, err := s.AttemptLockWithTTL(ctx, "short", []byte("v"), time.Minute)
	require.NoError(t, err)
	var doc struct {
		ExpiresAt time.Time `bson:"expires_at"`
	}
	require.NoError(t, coll.FindOne(ctx, bson.D{{Key: "_id", Value: "short"}}).Decode(&doc))
	require.WithinDuration(t, time.Now().Add(time.Minute), doc.ExpiresAt, 30*time.Second)

	require.NoError(t, s.Complete(ctx, "short", []byte("done"), []byte("v")))
	require.NoError(t, coll.FindOne(ctx, bson.D{{Key: "_id", Value: "short"}}).Decode(&doc))
	require.WithinDuration(t, time.Now().Add(time.Hour), doc.ExpiresAt, 30*time.Second, "a result keeps the configured TTL")
}

func TestStorageConcurrentLock(t *testing.T) {
	t.Parallel()
	s, _ := newStorage(t)

	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			ok, _, _, err := s.AttemptLock(t.Context(), "race", []byte("v"+strconv.Itoa(i)))
			if err == nil && ok {
				acquired.Add(1)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), acquired.Load(), "exactly one caller holds the lock")
}

func TestKeeperOnMongo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, _ := newStorage(t)
	k := idempotency.New(s, idempotency.WithMaxLockDuration(time.Minute))
	fp := idempotency.Fingerprint([]byte("create"), []byte("a"))

	ok, held, err := k.AttemptLockWithOpts(ctx, "k", idempotency.AttemptLockOpts{Fingerprint: fp})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(ctx, "k", "entity", held))

	_, st, err := k.AttemptLockWithOpts(ctx, "k", idempotency.AttemptLockOpts{Fingerprint: fp})
	require.NoError(t, err)
	require.Equal(t, idempotency.StatusSuccess, st.Status)
	require.Equal(t, "entity", st.Data)
	_, _, err = k.AttemptLockWithOpts(ctx, "k", idempotency.AttemptLockOpts{Fingerprint: []byte("other")})
	require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)
}

var errBusiness = errors.New("business rule failed")

// TestKeeperInTransaction pins that the lock commits and aborts with the
// caller's own writes.
func TestKeeperInTransaction(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	requireReplicaSet(t, db)
	s, err := mongostorage.New(db)
	require.NoError(t, err)
	k := idempotency.New(s, idempotency.WithMaxLockDuration(time.Minute))
	orders := db.Collection("orders")
	_, err = orders.InsertOne(t.Context(), bson.D{{Key: "_id", Value: "seed"}}) // the collection must exist before a transaction writes to it
	require.NoError(t, err)

	run := func(key string, fail bool) error {
		sess, err := db.Client().StartSession()
		require.NoError(t, err)
		defer sess.EndSession(context.WithoutCancel(t.Context()))
		_, err = sess.WithTransaction(t.Context(), func(ctx context.Context) (any, error) {
			ok, held, err := k.AttemptLock(ctx, key)
			if err != nil || !ok {
				return nil, err
			}
			if _, err := orders.InsertOne(ctx, bson.D{{Key: "_id", Value: key}}); err != nil {
				return nil, err
			}
			if fail {
				return nil, errBusiness
			}
			return nil, k.Complete(ctx, key, key, held)
		})
		return err
	}

	require.ErrorIs(t, run("aborted", true), errBusiness)
	ok, held, err := k.AttemptLock(t.Context(), "aborted")
	require.NoError(t, err)
	require.True(t, ok, "an aborted transaction leaves no lock behind")
	require.NoError(t, k.Release(t.Context(), "aborted", held))

	require.NoError(t, run("committed", false))
	_, st, err := k.AttemptLock(t.Context(), "committed")
	require.NoError(t, err)
	require.Equal(t, idempotency.StatusSuccess, st.Status)
	n, err := orders.CountDocuments(t.Context(), bson.D{{Key: "_id", Value: "committed"}})
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

// TestKeeperConcurrentTransactions pins that duplicates racing in their own
// transactions apply the command once: the loser's write conflicts, its
// transaction retries and then sees the committed key.
func TestKeeperConcurrentTransactions(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	requireReplicaSet(t, db)
	s, err := mongostorage.New(db)
	require.NoError(t, err)
	k := idempotency.New(s, idempotency.WithMaxLockDuration(time.Minute))
	ledger := db.Collection("ledger")
	_, err = ledger.InsertOne(t.Context(), bson.D{{Key: "_id", Value: "seed"}})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			sess, err := db.Client().StartSession()
			if err != nil {
				return
			}
			defer sess.EndSession(context.WithoutCancel(t.Context()))
			_, _ = sess.WithTransaction(t.Context(), func(ctx context.Context) (any, error) {
				ok, held, err := k.AttemptLock(ctx, "pay-1")
				if err != nil || !ok {
					return nil, err
				}
				if _, err := ledger.InsertOne(ctx, bson.D{{Key: "op", Value: "pay-1"}}); err != nil {
					return nil, err
				}
				return nil, k.Complete(ctx, "pay-1", nil, held)
			})
		})
	}
	wg.Wait()
	n, err := ledger.CountDocuments(t.Context(), bson.D{{Key: "op", Value: "pay-1"}})
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "the command is applied exactly once")
}

func TestStorageSameBytesNotAcquiredTwice(t *testing.T) {
	t.Parallel()
	s, _ := newStorage(t)

	ok, _, _, err := s.AttemptLock(t.Context(), "k", []byte("same"))
	require.NoError(t, err)
	require.True(t, ok)
	ok, existing, token, err := s.AttemptLock(t.Context(), "k", []byte("same"))
	require.NoError(t, err)
	require.False(t, ok, "a second caller with identical bytes does not acquire the held lock")
	require.Equal(t, []byte("same"), existing)
	require.Nil(t, token)
}

func TestStorageReleaseExpired(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, coll := newStorage(t)

	_, _, _, err := s.AttemptLock(ctx, "k", []byte("old"))
	require.NoError(t, err)
	_, err = coll.UpdateByID(ctx, "k", bson.D{{Key: "$set", Value: bson.D{{Key: "expires_at", Value: time.Now().Add(-time.Hour)}}}})
	require.NoError(t, err)
	require.ErrorIs(t, s.Release(ctx, "k", []byte("old")), storages.ErrLockStolen, "an expired lock is no longer the caller's")
}

func TestStorageCompleteWithoutToken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, coll := newStorage(t, mongostorage.WithTTL(time.Hour))

	// No CAS guard: the value is written whether or not the key exists.
	require.NoError(t, s.Complete(ctx, "fresh", []byte("done"), nil))
	_, existing, _, err := s.AttemptLock(ctx, "fresh", []byte("lock"))
	require.NoError(t, err)
	require.Equal(t, []byte("done"), existing)

	_, _, _, err = s.AttemptLock(ctx, "held", []byte("lock"))
	require.NoError(t, err)
	require.NoError(t, s.Complete(ctx, "held", []byte("overwritten"), nil))
	var doc struct {
		Value     []byte    `bson:"val"`
		ExpiresAt time.Time `bson:"expires_at"`
	}
	require.NoError(t, coll.FindOne(ctx, bson.D{{Key: "_id", Value: "held"}}).Decode(&doc))
	require.Equal(t, []byte("overwritten"), doc.Value)
	require.WithinDuration(t, time.Now().Add(time.Hour), doc.ExpiresAt, 30*time.Second)
}

func TestNewWithCollection(t *testing.T) {
	t.Parallel()
	coll := testDB(t).Collection("custom_keys")
	s, err := mongostorage.NewWithCollection(coll, mongostorage.WithCollectionName("ignored"))
	require.NoError(t, err)

	ok, _, _, err := s.AttemptLock(t.Context(), "k", []byte("v"))
	require.NoError(t, err)
	require.True(t, ok)
	n, err := coll.CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "the given collection is used")

	indexes, err := coll.Indexes().ListSpecifications(t.Context())
	require.NoError(t, err)
	var ttl bool
	for _, ix := range indexes {
		if ix.ExpireAfterSeconds != nil && *ix.ExpireAfterSeconds == 0 {
			ttl = true
		}
	}
	require.True(t, ttl, "the TTL index exists")
}
