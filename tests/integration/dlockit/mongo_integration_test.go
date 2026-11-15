// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/coord/dlock"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/providertest"
	"github.com/altessa-s/go-atlas/tests/integration/dlockit"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	lockmongo "github.com/altessa-s/go-atlas/coord/dlock/providers/mongo"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

const mongoLockTTL = 15 * time.Second

// mongoDB connects to the compose replica set and returns a throwaway
// database, dropped on cleanup. It skips when MongoDB is unreachable.
func mongoDB(tb testing.TB) *mongodrv.Database {
	tb.Helper()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(livebackend.MongoURI()).
		SetServerSelectionTimeout(3 * time.Second))
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.WithoutCancel(tb.Context()))
		// The URI is not printed: an override may carry a password.
		tb.Skipf("mongodb unreachable (%v) — start it with: make integration-up", err)
	}
	db := client.Database("dlockit_" + strings.ToLower(rand.Text()[:12]))
	tb.Cleanup(func() {
		_ = db.Drop(context.WithoutCancel(tb.Context()))
		_ = client.Disconnect(context.WithoutCancel(tb.Context()))
	})
	require.NoError(tb, db.CreateCollection(tb.Context(), lockmongo.DefaultCollection))
	require.NoError(tb, mongoLocker(tb, db).EnsureIndexes(tb.Context()))
	return db
}

// mongoLocker returns a locker over db with a fifteen-second TTL unless opts
// override it, closed on cleanup.
func mongoLocker(tb testing.TB, db *mongodrv.Database, opts ...lockmongo.Option) *lockmongo.Locker {
	tb.Helper()
	l, err := lockmongo.New(db, append([]lockmongo.Option{lockmongo.WithTTL(mongoLockTTL)}, opts...)...)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = l.Close(context.WithoutCancel(tb.Context())) })
	return l
}

// lockDoc returns the stored lock document of key.
func lockDoc(tb testing.TB, db *mongodrv.Database, key string) bson.M {
	tb.Helper()
	var doc bson.M
	require.NoError(tb, db.Collection(lockmongo.DefaultCollection).FindOne(tb.Context(), bson.M{"_id": key}).Decode(&doc))
	return doc
}

// TestMongoProviderContract runs the provider contract suite on MongoDB, each
// contract over its own database. Expire ages a lease out by writing
// expires_at directly, as a holder that stopped renewing would leave it.
func TestMongoProviderContract(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(tb testing.TB) providertest.Backend {
		db := mongoDB(tb)
		ttl := mongoLockTTL
		if strings.HasSuffix(tb.Name(), "/Renewal") || strings.HasSuffix(tb.Name(), "/ExpiredLeaseStaysLost") {
			ttl = time.Second
		}
		return providertest.Backend{
			NewProvider: func(tb testing.TB) providers.Provider { return mongoLocker(tb, db, lockmongo.WithTTL(ttl)) },
			Expire: func(tb testing.TB, key string) {
				res, err := db.Collection(lockmongo.DefaultCollection).UpdateOne(tb.Context(),
					bson.M{"_id": key}, bson.M{"$set": bson.M{"expires_at": time.Unix(0, 0)}})
				require.NoError(tb, err)
				require.Equal(tb, int64(1), res.MatchedCount, "no lock document for %q", key)
			},
		}
	})
}

// TestMongoContendersNeverOverlap races several dlock instances, each over its
// own Mongo locker, for one key and records every critical section.
func TestMongoContendersNeverOverlap(t *testing.T) {
	t.Parallel()
	const contenders, rounds = 6, 3
	db := mongoDB(t)
	var (
		section dlockit.Critical
		wg      sync.WaitGroup
	)
	for i := range contenders {
		dl := dlock.New(mongoLocker(t, db))
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
}

// TestMongoReleaseKeepsTheDocument asserts a release ends the lease but keeps
// the document and its fencing token, so the next holder continues the
// sequence instead of restarting it at 1.
func TestMongoReleaseKeepsTheDocument(t *testing.T) {
	t.Parallel()
	db := mongoDB(t)
	l := mongoLocker(t, db)

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	info, err := lk.GetLockInfo(t.Context())
	require.NoError(t, err)
	require.NoError(t, lk.Release(t.Context()))

	doc := lockDoc(t, db, "k")
	require.Empty(t, doc["owner"], "a released lease has no owner")
	require.EqualValues(t, info.FencingToken, doc["fencing"], "the fencing token survives the release")
	expires, ok := doc["expires_at"].(bson.DateTime)
	require.True(t, ok, "expires_at is a date: %T", doc["expires_at"])
	require.False(t, expires.Time().After(time.Now().Add(time.Second)), "a released lease ends at once")
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)

	next, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	t.Cleanup(func() { _ = next.Release(context.WithoutCancel(t.Context())) })
	nextInfo, err := next.GetLockInfo(t.Context())
	require.NoError(t, err)
	require.Equal(t, info.FencingToken+1, nextInfo.FencingToken)
}

// TestMongoHardLeaseEndsWhileHeld asserts a lease taken WithoutRenewal is not
// extended: another locker takes the key one TTL later although the first
// holder never released it, and the first holder then sees its lock lost.
func TestMongoHardLeaseEndsWhileHeld(t *testing.T) {
	t.Parallel()
	db := mongoDB(t)
	const ttl = time.Second
	a := mongoLocker(t, db, lockmongo.WithoutRenewal(), lockmongo.WithTTL(ttl))
	b := mongoLocker(t, db, lockmongo.WithoutRenewal(), lockmongo.WithTTL(ttl))

	var held providers.Lock
	var err error
	require.Eventually(t, func() bool {
		held, err = a.Lock(t.Context(), "k")
		if errors.Is(err, providers.ErrLockNotHeld) {
			return false
		}
		require.NoError(t, err)
		return true
	}, 5*time.Second, 20*time.Millisecond)
	_, err = b.Lock(t.Context(), "k")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)

	var taken providers.Lock
	require.Eventually(t, func() bool {
		taken, err = b.Lock(t.Context(), "k")
		return err == nil
	}, 5*ttl, 50*time.Millisecond, "the hard lease must end one TTL after it was taken")
	t.Cleanup(func() { _ = taken.Release(context.WithoutCancel(t.Context())) })

	_, err = held.GetLockInfo(t.Context())
	require.ErrorIs(t, err, providers.ErrLockNotHeld, "the first holder's lease has ended")
	require.NoError(t, held.Release(t.Context()))
	_, err = taken.GetLockInfo(t.Context())
	require.NoError(t, err, "the ended holder's release must not end the new lease")
}

// cleanupIndexExpiry returns the expireAfterSeconds of the cleanup TTL index
// on expires_at, or false when the collection has no such index.
func cleanupIndexExpiry(tb testing.TB, db *mongodrv.Database) (int32, bool) {
	tb.Helper()
	specs, err := db.Collection(lockmongo.DefaultCollection).Indexes().ListSpecifications(tb.Context())
	require.NoError(tb, err)
	i := slices.IndexFunc(specs, func(s mongodrv.IndexSpecification) bool { return s.ExpireAfterSeconds != nil })
	if i < 0 {
		return 0, false
	}
	_, err = specs[i].KeysDocument.LookupErr("expires_at")
	require.NoError(tb, err, "the TTL index is on expires_at: %s", specs[i].KeysDocument)
	return *specs[i].ExpireAfterSeconds, true
}

// TestMongoExpiredCleanupIndexLifecycle walks the cleanup TTL index through
// the configurations a deployment goes through: created with the grace
// rounded up to whole seconds, idempotent, updated in place when the grace
// changes, and dropped when cleanup is turned off.
func TestMongoExpiredCleanupIndexLifecycle(t *testing.T) {
	t.Parallel()
	db := mongoDB(t)
	// The collection must exist for the index listing of the last step.
	_, err := mongoLocker(t, db).Lock(t.Context(), "k")
	require.NoError(t, err)

	steps := []struct {
		name   string
		opts   []lockmongo.Option
		want   int32
		exists bool
	}{
		{"created, grace rounded up", []lockmongo.Option{lockmongo.WithExpiredCleanup(1500 * time.Millisecond)}, 2, true},
		{"unchanged grace is a no-op", []lockmongo.Option{lockmongo.WithExpiredCleanup(2 * time.Second)}, 2, true},
		{"changed grace updates in place", []lockmongo.Option{lockmongo.WithExpiredCleanup(time.Hour)}, 3600, true},
		{"cleanup off drops the index", nil, 0, false},
		{"already dropped is a no-op", nil, 0, false},
	}
	for _, step := range steps {
		l := mongoLocker(t, db, step.opts...)
		for range 2 {
			require.NoError(t, l.EnsureIndexes(t.Context()), step.name)
		}
		got, exists := cleanupIndexExpiry(t, db)
		require.Equal(t, step.exists, exists, step.name)
		require.Equal(t, step.want, got, step.name)
	}
}
