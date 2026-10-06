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

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/mongo"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestNew_Validation(t *testing.T) {
	t.Parallel()
	_, err := mongo.New(nil)
	require.Error(t, err)

	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	for _, opts := range [][]mongo.Option{
		{mongo.WithRenewRatio(1)},
		{mongo.WithRenewRatio(1e-20)},
		{mongo.WithTTL(500 * time.Microsecond)},
		{mongo.WithTTL(2 * time.Millisecond), mongo.WithRenewRatio(0.1)},
	} {
		_, err = mongo.New(client.Database("x"), opts...)
		require.Error(t, err)
	}
	l, err := mongo.New(client.Database("x"))
	require.NoError(t, err)
	require.NotNil(t, l)
}

// newIT returns a database on a live MongoDB (MONGO_URI), dropped on cleanup,
// or skips the test.
func newIT(t *testing.T) *mongodrv.Database {
	t.Helper()
	uri, explicit := os.LookupEnv("MONGO_URI")
	if !explicit {
		uri = "mongodb://127.0.0.1:27017"
	}
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	if err := client.Ping(t.Context(), nil); err != nil {
		// A configured MongoDB must be reachable: the run asked for these tests.
		require.False(t, explicit, "MONGO_URI is set but MongoDB is unreachable: %v", err)
		t.Skipf("mongodb not reachable at %s: %v", uri, err)
	}
	db := client.Database("dlock_it_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(dbSeq.Add(1), 10))
	if err := db.Collection("probe").Drop(t.Context()); err != nil {
		require.False(t, explicit, "MONGO_URI is set but MongoDB is not usable: %v", err)
		t.Skipf("mongodb not usable at %s: %v", uri, err)
	}
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return db
}

func newLocker(t *testing.T, db *mongodrv.Database, opts ...mongo.Option) *mongo.Locker {
	t.Helper()
	l, err := mongo.New(db, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	return l
}

func fencing(t *testing.T, lk providers.Lock) uint64 {
	t.Helper()
	info, err := lk.GetLockInfo(t.Context())
	require.NoError(t, err)
	return info.FencingToken
}

func TestIntegration_ConcurrentLockHasOneWinner(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			lk, err := l.Lock(t.Context(), "race")
			if err == nil {
				wins.Add(1)
				t.Cleanup(func() { _ = lk.Release(context.Background()) })
				return
			}
			require.ErrorIs(t, err, errs.ErrLockNotHeld)
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), wins.Load())
}

func TestIntegration_ReleaseHandsOverWithHigherFencing(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	a, b := newLocker(t, db), newLocker(t, db)

	lkA, err := a.Lock(t.Context(), "k")
	require.NoError(t, err)
	first := fencing(t, lkA)
	_, err = b.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)

	require.NoError(t, lkA.Release(t.Context()))
	_, err = a.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)

	lkB, err := b.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.Greater(t, fencing(t, lkB), first)
	require.NoError(t, lkB.Release(t.Context()))
}

// A holder whose lease expired (a stalled process) loses the key: another
// holder takes it with a higher fencing token, and the stale holder can
// neither renew nor release the new lease.
func TestIntegration_ExpiredLeaseIsTakenOver(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	a := newLocker(t, db, mongo.WithTTL(3*time.Second))
	b := newLocker(t, db, mongo.WithTTL(3*time.Second))

	lkA, err := a.Lock(t.Context(), "k")
	require.NoError(t, err)
	first := fencing(t, lkA)

	// Expire A's lease as if A had stopped renewing.
	_, err = db.Collection(mongo.DefaultCollection).UpdateOne(t.Context(), bson.M{"_id": "k"},
		bson.M{"$set": bson.M{"expires_at": time.Now().Add(-time.Minute)}})
	require.NoError(t, err)

	lkB, err := b.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, first+1, fencing(t, lkB))
	_, err = lkA.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the stale holder must not see the new holder's token")

	require.NoError(t, lkA.Release(t.Context()))
	info, err := b.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the stale holder must not release the new lease")
	require.Equal(t, first+1, info.FencingToken)

	// B keeps renewing past its TTL.
	time.Sleep(4 * time.Second)
	info, err = b.GetLockInfo(t.Context(), "k")
	require.NoError(t, err)
	require.True(t, info.LastRenewed.After(info.AcquiredAt), "the lease must have been renewed")
	require.NoError(t, lkB.Release(t.Context()))
}

func TestIntegration_ContextEndReleases(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db)

	ctx, cancel := context.WithCancel(t.Context())
	_, err := l.Lock(ctx, "k")
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		_, err := l.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 5*time.Second, 20*time.Millisecond)
	_, err = l.Lock(t.Context(), "k")
	require.NoError(t, err)
}

func TestIntegration_CloseReleasesAndRejects(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l, err := mongo.New(db)
	require.NoError(t, err)
	require.NoError(t, l.Probe(t.Context()))

	_, err = l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.NoError(t, l.Close(t.Context()))
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = l.Lock(t.Context(), "k")
	require.Error(t, err)
	require.Error(t, l.Probe(t.Context()))
}

// A release that failed (here: its context already ended) leaves the lease
// in place and can be retried.
func TestIntegration_FailedReleaseIsRetryable(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db)

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, lk.Release(canceled))
	_, err = l.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "a failed release keeps the lease")

	require.NoError(t, lk.Release(t.Context()))
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// Locks acquired while Close runs are released either by Close or by Lock
// itself: none is left held.
func TestIntegration_CloseRacingLock(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l, err := mongo.New(db)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			lk, err := l.Lock(context.Background(), "k-"+strconv.Itoa(i))
			if err == nil {
				_ = lk // released by Close
			}
		})
	}
	require.NoError(t, l.Close(t.Context()))
	wg.Wait()

	other := newLocker(t, db)
	for i := range 20 {
		_, err := other.GetLockInfo(t.Context(), "k-"+strconv.Itoa(i))
		require.ErrorIs(t, err, errs.ErrLockNotHeld, "k-%d", i)
	}
}

// A lease that expires without being taken over stays lost: renewal does not
// revive it, and the next acquisition gets a new fencing token.
func TestIntegration_ExpiredLeaseStaysLost(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db, mongo.WithTTL(3*time.Second))

	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	first := fencing(t, lk)
	_, err = db.Collection(mongo.DefaultCollection).UpdateOne(t.Context(), bson.M{"_id": "k"},
		bson.M{"$set": bson.M{"expires_at": time.Now().Add(-time.Minute)}})
	require.NoError(t, err)

	time.Sleep(1500 * time.Millisecond) // past the next renewal
	_, err = lk.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "renewal must not revive an expired lease")

	other, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, first+1, fencing(t, other))
}

var errAbort = errors.New("abort transaction")

// Lock operations stay out of a caller's MongoDB transaction: a lock taken
// inside a transaction that aborts is still held, and its fencing token is
// never reissued.
func TestIntegration_LockIgnoresCallerTransaction(t *testing.T) {
	t.Parallel()
	db := newIT(t)
	l := newLocker(t, db)

	sess, err := db.Client().StartSession()
	require.NoError(t, err)
	t.Cleanup(func() { sess.EndSession(context.Background()) })

	var lk providers.Lock
	var token uint64
	_, err = sess.WithTransaction(t.Context(), func(sc context.Context) (any, error) {
		var lockErr error
		lk, lockErr = l.Lock(sc, "k")
		if lockErr != nil {
			return nil, lockErr
		}
		info, infoErr := lk.GetLockInfo(sc)
		if infoErr != nil {
			return nil, infoErr
		}
		token = info.FencingToken
		return nil, errAbort
	})
	require.ErrorIs(t, err, errAbort)

	info, err := l.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the lock must survive the aborted transaction")
	require.Equal(t, token, info.FencingToken)
	require.NoError(t, lk.Release(t.Context()))

	next, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, token+1, fencing(t, next), "a fencing token must never be reissued")
}

// dbSeq makes database names unique between parallel tests: the wall clock
// alone repeats within its resolution (a microsecond on macOS).
var dbSeq atomic.Int64
