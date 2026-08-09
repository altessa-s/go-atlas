// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
	natsio "github.com/nats-io/nats.go"
)

// natsLocker builds a locker on a bucket of its own, over a shared embedded
// server so several lockers in one test can contend for the same key.
func natsLocker(tb testing.TB, nc *natsio.Conn, bucket string, opts ...locknats.Option) *locknats.Locker {
	tb.Helper()

	locker, err := locknats.New(tb.Context(), nc, append([]locknats.Option{locknats.WithBucket(bucket)}, opts...)...)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = locker.Close(context.Background()) })

	return locker
}

// bucketName derives a throwaway bucket name from the test name.
func bucketName(tb testing.TB) string {
	tb.Helper()

	return strings.ReplaceAll(tb.Name(), "/", "-")
}

// TestSynchronize_HoldsLockForTheWholeCallback pins that the lock outlives the
// *acquire* timeout.
//
// The acquire timeout bounds how long we wait to get the lock, not how long we
// may hold it. Running the renewal on the context that carries it meant the
// lease was dropped the moment the timeout elapsed, while the callback was
// still inside its critical section.
func TestSynchronize_HoldsLockForTheWholeCallback(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	dl := dlock.New(natsLocker(t, nc, bucketName(t), locknats.WithAcquireTimeout(2*time.Second)))

	var heldThroughout atomic.Bool

	err := dl.Synchronize(t.Context(), "resource", func(context.Context) error {
		time.Sleep(4 * time.Second) // outlive the acquire timeout

		_, infoErr := dl.GetLockInfo(context.Background(), "resource")
		heldThroughout.Store(infoErr == nil)

		return nil
	})
	require.NoError(t, err)

	require.True(t, heldThroughout.Load(),
		"the lock was released while the callback was still running")
}

// TestSynchronize_ExcludesASecondHolder pins the consequence of the above, and
// the whole point of the package: nobody else gets the key while a callback
// holds it.
func TestSynchronize_ExcludesASecondHolder(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	bucket := bucketName(t)
	holder := dlock.New(natsLocker(t, nc, bucket, locknats.WithAcquireTimeout(2*time.Second)))
	contender := natsLocker(t, nc, bucket)

	var intruded atomic.Bool

	err := holder.Synchronize(t.Context(), "resource", func(context.Context) error {
		deadline := time.Now().Add(5 * time.Second) // well past the acquire timeout
		for time.Now().Before(deadline) {
			if lk, lockErr := contender.Lock(context.Background(), "resource"); lockErr == nil {
				intruded.Store(true)
				_ = lk.Release(context.Background())

				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}

		return nil
	})
	require.NoError(t, err)

	require.False(t, intruded.Load(),
		"a second holder acquired the lock while the first was inside Synchronize")
}

// TestLock_TTLAboveTheDefaultStillHolds pins that a configured lock TTL is the
// duration that actually governs. The bucket's key TTL is what reaps an
// abandoned lock, so pinning it to a constant while the lock TTL came from
// options produced a lock the server aged out early — the renewal was
// scheduled off the configured TTL and the key was gone before it fired.
func TestLock_TTLAboveTheDefaultStillHolds(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	locker := natsLocker(t, nc, bucketName(t), locknats.WithTTL(60*time.Second))

	lk, err := locker.Lock(t.Context(), "resource")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lk.Release(context.Background()) })

	time.Sleep(locknats.DefaultBucketKeysTTL + 3*time.Second)

	_, err = locker.GetLockInfo(context.Background(), "resource")
	require.NoError(t, err, "the lock key expired while its holder still believed it had the lease")
}

// TestRelease_StopsTheRenewalGoroutine pins that releasing a lock ends its
// renewal immediately. Leaving the goroutine to discover the release on its
// next tick piles up one live goroutine per released lock for up to a renewal
// interval, and reports the voluntary release as a lost lease.
func TestRelease_StopsTheRenewalGoroutine(t *testing.T) {
	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	locker := natsLocker(t, nc, bucketName(t))

	// Serial: counting a named frame across every goroutine in the process
	// would be meaningless with another lock camping in parallel.
	camping := func() int {
		buf := make([]byte, 1<<20)
		return strings.Count(string(buf[:runtime.Stack(buf, true)]), "natskvlease.(*Lease).campingLoop")
	}

	before := camping()

	lk, err := locker.Lock(t.Context(), "resource")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return camping() == before+1 },
		2*time.Second, 10*time.Millisecond, "a held lock should have exactly one renewal goroutine")

	require.NoError(t, lk.Release(context.Background()))

	require.Eventually(t, func() bool { return camping() == before },
		2*time.Second, 10*time.Millisecond, "the renewal goroutine outlived Release")
}

// TestLock_RenewRatioIsHonored pins that WithRenewRatio reaches the lease. The
// provider used to compute a renewal interval from it, hand it to a function
// that discarded the argument, and build the lease with the package default —
// so the option changed nothing.
func TestLock_RenewRatioIsHonored(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	// 4s TTL at ratio 0.05 renews every 200ms, so a wired-up ratio bumps the
	// revision several times inside the window below.
	locker := natsLocker(t, nc, bucketName(t), locknats.WithTTL(4*time.Second), locknats.WithRenewRatio(0.05))

	lk, err := locker.Lock(t.Context(), "resource")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lk.Release(context.Background()) })

	first, err := lk.GetLockInfo(t.Context())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		info, infoErr := lk.GetLockInfo(context.Background())
		return infoErr == nil && info.FencingToken > first.FencingToken
	}, 2*time.Second, 50*time.Millisecond, "no renewal happened — WithRenewRatio is not reaching the lease")
}
