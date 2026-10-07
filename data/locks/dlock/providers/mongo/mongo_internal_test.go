// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// liveDB returns a database on MONGO_URI, dropped on cleanup, or skips.
func liveDB(t *testing.T) *mongodrv.Database {
	t.Helper()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set")
	}
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	require.NoError(t, client.Ping(t.Context(), nil), "MONGO_URI is set but MongoDB is unreachable")
	db := client.Database("dlock_int_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(dbSeq.Add(1), 10))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return db
}

// The TTL is stored in whole milliseconds, so every deadline uses it
// truncated the same way.
func TestNew_TruncatesTTLToMilliseconds(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	l, err := New(client.Database("x"), WithTTL(2900*time.Microsecond), WithRenewRatio(0.9))
	require.NoError(t, err)
	require.Equal(t, 2*time.Millisecond, l.s.ttl)
	require.Less(t, l.engine.Interval(), l.s.ttl)
}

// Close must not return while an acquisition that started before it can still
// leave a lease behind.
func TestClose_WaitsForInflightAcquisition(t *testing.T) {
	t.Parallel()
	db := liveDB(t)
	l, err := New(db)
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	l.engine.AfterAcquire = func() {
		close(entered)
		<-release
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := l.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered

	closed := make(chan error, 1)
	go func() { closed <- l.Close(context.Background()) }()
	select {
	case <-closed:
		t.Fatal("Close returned while an acquisition was in flight")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-closed)
	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "Close leaves no lease behind")
}

// An acquisition whose reply arrives after the lease's TTL has elapsed may
// already belong to another holder: Lock must reject it and release it.
func TestLock_LateAcquisitionIsRejected(t *testing.T) {
	t.Parallel()
	db := liveDB(t)
	l, err := New(db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	// The first reading is the request; every later one is a full TTL on.
	base := time.Now()
	var calls atomic.Int32
	l.engine.Now = func() time.Time {
		if calls.Add(1) == 1 {
			return base
		}
		return base.Add(l.s.ttl)
	}

	_, err = l.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the late acquisition must be released")
}

// A Close that times out while an acquisition is in flight still stops the
// renewal of every held lock, so their leases lapse; a later Close finishes
// the cleanup.
func TestClose_TimeoutStopsRenewalAndIsRetryable(t *testing.T) {
	t.Parallel()
	db := liveDB(t)
	l, err := New(db, WithTTL(2*time.Second))
	require.NoError(t, err)

	_, err = l.Lock(context.Background(), "held")
	require.NoError(t, err)

	entered, release := make(chan struct{}), make(chan struct{})
	l.engine.AfterAcquire = func() {
		close(entered)
		<-release
	}
	lockErr := make(chan error, 1)
	go func() {
		_, err := l.Lock(context.Background(), "inflight")
		lockErr <- err
	}()
	<-entered

	short, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	require.Error(t, l.Close(short), "Close cannot finish while the acquisition is blocked")

	// Renewal stopped: the held lease lapses after its TTL.
	require.Eventually(t, func() bool {
		_, err := l.GetLockInfo(t.Context(), "held")
		return err != nil
	}, 6*time.Second, 50*time.Millisecond)

	close(release)
	// Blocked past its TTL the acquisition is late, otherwise the provider
	// closed; either way no lock is returned and the lease is released.
	err = <-lockErr
	require.True(t, errors.Is(err, leasing.ErrClosed) || errors.Is(err, errs.ErrLockNotHeld), "got %v", err)
	require.NoError(t, l.Close(t.Context()))
	_, err = l.GetLockInfo(t.Context(), "inflight")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// When no renewal is confirmed within the lease on this process's clock, the
// lock releases the lease on the server instead of abandoning it.
func TestRenew_UnconfirmedLeaseIsReleased(t *testing.T) {
	t.Parallel()
	db := liveDB(t)
	l, err := New(db, WithTTL(3*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	var offset atomic.Int64
	l.engine.Now = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }

	_, err = l.Lock(context.Background(), "k")
	require.NoError(t, err)
	// This process's clock jumps past the lease: the next renewal finds its
	// deadline missed while the server lease is still live.
	offset.Store(int64(4 * time.Second))
	require.Eventually(t, func() bool {
		_, err := l.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 2*time.Second, 20*time.Millisecond, "the live server lease must be released, not left to expire")
}

// An acquisition that completes after Close began and cannot be released at
// once stays registered, so a later Close releases it.
func TestClose_RetriesFailedCleanupOfRacingAcquisition(t *testing.T) {
	t.Parallel()
	db := liveDB(t)
	l, err := New(db)
	require.NoError(t, err)

	entered, release := make(chan struct{}), make(chan struct{})
	l.engine.AfterAcquire = func() {
		close(entered)
		<-release
	}
	var faults atomic.Int32
	errFault := errors.New("release fault")
	l.engine.ReleaseFault = func() error {
		if faults.Add(1) <= 2 {
			return errFault
		}
		return nil
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := l.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- l.Close(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // let Close mark the provider closed
	close(release)

	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	require.ErrorIs(t, <-closed, errFault, "the retained acquisition's release failed again")
	_, err = l.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the lease is still live")

	require.NoError(t, l.Close(t.Context()), "a later Close retries the release")
	_, err = l.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// dbSeq makes database names unique between parallel tests: the wall clock
// alone repeats within its resolution (a microsecond on macOS).
var dbSeq atomic.Int64

// An acquisition applied by MongoDB whose reply is lost is released by Lock;
// when that release fails too, Close retries it.
func TestLock_AmbiguousAcquisitionIsReleased(t *testing.T) {
	t.Parallel()
	errLost := errors.New("reply lost")

	t.Run("released_by_lock", func(t *testing.T) {
		t.Parallel()
		l, err := New(liveDB(t))
		require.NoError(t, err)
		t.Cleanup(func() { _ = l.Close(context.Background()) })
		l.engine.AcquireFault = func() error { return errLost }

		_, err = l.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = l.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})

	t.Run("retried_by_close", func(t *testing.T) {
		t.Parallel()
		l, err := New(liveDB(t))
		require.NoError(t, err)
		l.engine.AcquireFault = func() error { return errLost }
		var faults atomic.Int32
		l.engine.ReleaseFault = func() error {
			if faults.Add(1) == 1 {
				return errLost
			}
			return nil
		}

		_, err = l.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = l.GetLockInfo(t.Context(), "k")
		require.NoError(t, err, "the applied lease is still live")
		require.NoError(t, l.Close(t.Context()))
		_, err = l.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})
}

// A Release waiting behind a slow concurrent release honors its own deadline.
func TestRelease_ConcurrentHonorsDeadline(t *testing.T) {
	t.Parallel()
	l, err := New(liveDB(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	lk, err := l.Lock(t.Context(), "k")
	require.NoError(t, err)

	entered, unblock := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	l.engine.ReleaseFault = func() error {
		if calls.Add(1) == 1 {
			close(entered)
			<-unblock
		}
		return nil
	}
	first := make(chan error, 1)
	go func() { first <- lk.Release(context.Background()) }()
	<-entered

	short, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	began := time.Now()
	require.ErrorIs(t, lk.Release(short), context.DeadlineExceeded)
	require.Less(t, time.Since(began), time.Second, "the second Release must not wait for the first")

	close(unblock)
	require.NoError(t, <-first)
}
