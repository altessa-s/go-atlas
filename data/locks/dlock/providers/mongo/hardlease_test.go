// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/mongo"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestNew_HardLeaseAndCleanupValidation(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("x")

	_, err = mongo.New(db, mongo.WithExpiredCleanup(-time.Second))
	require.Error(t, err, "a negative cleanup grace is refused")
	_, err = mongo.New(db, mongo.WithExpiredCleanup(0))
	require.NoError(t, err, "a zero grace deletes documents as soon as their lease ended")
	_, err = mongo.New(db, mongo.WithoutRenewal(), mongo.WithRenewRatio(1))
	require.NoError(t, err, "the renew ratio plays no part in a hard lease")
	_, err = mongo.New(db, mongo.WithoutRenewal(), mongo.WithTTL(500*time.Microsecond))
	require.Error(t, err, "a hard lease still needs a TTL of at least 1ms")
	_, err = mongo.New(db, mongo.WithoutRenewal(), mongo.WithTTL(time.Millisecond))
	require.NoError(t, err, "a hard lease needs no renewal interval")
}

// A hard lease is never renewed: the document keeps its acquisition time and
// another locker takes the key one TTL later.
func TestIntegration_HardLeaseIsNotRenewed(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	const ttl = 1500 * time.Millisecond
	a := newLocker(t, db, mongo.WithTTL(ttl), mongo.WithoutRenewal())
	b := newLocker(t, db, mongo.WithTTL(ttl), mongo.WithoutRenewal())

	lkA, err := a.Lock(t.Context(), "k")
	require.NoError(t, err)
	first := fencing(t, lkA)
	_, err = b.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)

	var lkB interface{ Release(context.Context) error }
	require.Eventually(t, func() bool {
		lk, err := b.Lock(t.Context(), "k")
		if err != nil {
			return false
		}
		lkB = lk
		return true
	}, 5*time.Second, 50*time.Millisecond, "the key is free once the hard lease ended")

	info, err := b.GetLockInfo(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, first+1, info.FencingToken)
	require.Equal(t, info.AcquiredAt, info.LastRenewed, "a hard lease is never renewed")
	_, err = lkA.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	require.NoError(t, lkA.Release(t.Context()))
	_, err = b.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the ended holder's release must not end the new lease")
	require.NoError(t, lkB.Release(t.Context()))
}

// cleanupIndex returns the expiry of the cleanup TTL index, or false when the
// collection has none.
func cleanupIndex(t *testing.T, db *mongodrv.Database) (int32, bool) {
	t.Helper()
	specs, err := db.Collection(mongo.DefaultCollection).Indexes().ListSpecifications(t.Context())
	require.NoError(t, err)
	for _, spec := range specs {
		if spec.Name == "expires_at_ttl" {
			require.Equal(t, `{"expires_at": {"$numberInt":"1"}}`, bson.Raw(spec.KeysDocument).String())
			require.NotNil(t, spec.ExpireAfterSeconds)
			return *spec.ExpireAfterSeconds, true
		}
	}
	return 0, false
}

// EnsureIndexes creates the TTL index with the grace rounded up to seconds,
// updates it when the grace changes, is idempotent, and drops it once cleanup
// is turned off.
func TestIntegration_EnsureIndexesReconcilesCleanup(t *testing.T) {
	t.Parallel()
	db := newIT(t)

	none := newLocker(t, db)
	require.NoError(t, none.EnsureIndexes(t.Context()), "no cleanup, no index: nothing to do")
	_, found := cleanupIndex(t, db)
	require.False(t, found)

	l := newLocker(t, db, mongo.WithExpiredCleanup(1500*time.Millisecond))
	require.NoError(t, l.EnsureIndexes(t.Context()))
	require.NoError(t, l.EnsureIndexes(t.Context()), "idempotent")
	seconds, found := cleanupIndex(t, db)
	require.True(t, found)
	require.EqualValues(t, 2, seconds, "the grace is rounded up to whole seconds")

	require.NoError(t, newLocker(t, db, mongo.WithExpiredCleanup(time.Minute)).EnsureIndexes(t.Context()))
	seconds, found = cleanupIndex(t, db)
	require.True(t, found)
	require.EqualValues(t, 60, seconds, "a changed grace updates the index in place")

	require.NoError(t, newLocker(t, db, mongo.WithExpiredCleanup(0)).EnsureIndexes(t.Context()))
	seconds, _ = cleanupIndex(t, db)
	require.Zero(t, seconds)

	require.NoError(t, none.EnsureIndexes(t.Context()))
	_, found = cleanupIndex(t, db)
	require.False(t, found, "turning cleanup off drops the index")
	require.NoError(t, none.EnsureIndexes(t.Context()))
}

// Replicas turning cleanup off together race to drop the TTL index; the ones
// that find it already gone (IndexNotFound) succeed as well.
func TestIntegration_EnsureIndexesConcurrentDrop(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	require.NoError(t, newLocker(t, db, mongo.WithExpiredCleanup(time.Minute)).EnsureIndexes(t.Context()))

	none := newLocker(t, db)
	const replicas = 8
	require.NoError(t, concurrency.Process(t.Context(), make([]int, replicas),
		func(ctx context.Context, _ int) error { return none.EnsureIndexes(ctx) },
		concurrency.WithConcurrency[int](replicas),
	))
	_, found := cleanupIndex(t, db)
	require.False(t, found)
}

// A released lease keeps its document with an expiry in the past, which the
// cleanup TTL index then deletes; the next acquisition of a deleted key
// starts again at fencing token 1.
func TestIntegration_CleanupRestartsFencing(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db, mongo.WithExpiredCleanup(0))
	require.NoError(t, l.EnsureIndexes(t.Context()))

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.EqualValues(t, 1, fencing(t, lk))
	require.NoError(t, lk.Release(t.Context()))

	var doc struct {
		ExpiresAt time.Time `bson:"expires_at"`
	}
	require.NoError(t, db.Collection(mongo.DefaultCollection).FindOne(t.Context(), bson.M{"_id": "k"}).Decode(&doc))
	require.False(t, doc.ExpiresAt.After(time.Now()), "a released lease is expired, so the TTL index may delete it")

	// Stand in for the TTL monitor, which runs about once a minute.
	_, err = db.Collection(mongo.DefaultCollection).DeleteOne(t.Context(), bson.M{"_id": "k"})
	require.NoError(t, err)
	lk, err = l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.EqualValues(t, 1, fencing(t, lk), "fencing is monotonic only within the grace")
}
