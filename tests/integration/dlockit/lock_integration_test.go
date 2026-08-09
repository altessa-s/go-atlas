// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlockit_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/tests/integration/dlockit"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

const (
	// lockTTL is the lease this suite locks with, and with it the bucket's key
	// TTL: it bounds how long an abandoned lock stays held.
	lockTTL = 3 * time.Second

	// holdTime is how long a contender stays inside the critical section. It
	// spans several renewal intervals, so a lock that stops being renewed is
	// caught while its holder is still inside.
	holdTime = 1500 * time.Millisecond
)

// natsURL returns the broker address, matching tests/integration/docker-compose.yml.
func natsURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// uniqueSuffix names a throwaway bucket so two runs against the same server —
// or two tests in one run — never collide.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// connect dials the broker, skipping the test when it is unreachable so a
// developer without the compose stack running still gets a green build.
func connect(tb testing.TB) *nats.Conn {
	tb.Helper()

	nc, err := nats.Connect(natsURL(),
		nats.Timeout(2*time.Second),
		nats.RetryOnFailedConnect(false),
		// The suite kills connections on purpose; reconnecting behind its back
		// would mask the expiry it is measuring.
		nats.NoReconnect(),
	)
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d nats", natsURL(), err)
	}
	tb.Cleanup(nc.Close)

	return nc
}

// locker builds a provider on its own connection against the given bucket.
func locker(tb testing.TB, bucket string, opts ...locknats.Option) (*locknats.Locker, *nats.Conn) {
	tb.Helper()

	nc := connect(tb)
	opts = append([]locknats.Option{locknats.WithBucket(bucket), locknats.WithTTL(lockTTL)}, opts...)

	prov, err := locknats.New(tb.Context(), nc, opts...)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = prov.Close(context.Background()) })

	return prov, nc
}

// TestLock_ContendersNeverOverlap is the property the package exists to
// provide: many holders racing for one key through a real broker, and no two
// of their critical sections ever coincide.
//
// Each contender holds for longer than a renewal interval, so a lock that
// silently stops being renewed shows up as a breach rather than passing.
func TestLock_ContendersNeverOverlap(t *testing.T) {
	t.Parallel()

	const (
		contenders = 6
		rounds     = 3
	)

	bucket := "contend-" + uniqueSuffix()
	key := "resource"

	var section dlockit.Critical
	var wg sync.WaitGroup

	for i := range contenders {
		prov, _ := locker(t, bucket)
		dl := dlock.New(prov)
		holder := fmt.Sprintf("holder-%d", i)

		wg.Go(func() {
			for range rounds {
				synchronizeUntilDone(t, dl, key, func(context.Context) error {
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
	require.NotEmpty(t, section.Visits(), "no contender ever entered the critical section")
}

// synchronizeUntilDone retries Synchronize until it gets a turn.
//
// Acquisition fails fast: the NATS provider makes one attempt and returns
// [errs.ErrLockNotHeld] when the key is taken — it does not wait for the holder
// to finish. Callers that need to be serialized rather than rejected have to
// loop, so the suite loops too.
func synchronizeUntilDone(tb testing.TB, dl *dlock.DLock, key string, fn func(context.Context) error) {
	tb.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		err := dl.Synchronize(tb.Context(), key, fn)
		if err == nil {
			return
		}
		if !errors.Is(err, errs.ErrLockNotHeld) {
			require.NoError(tb, err, "unexpected failure acquiring %q", key)
		}
		time.Sleep(20 * time.Millisecond)
	}

	tb.Fatalf("never acquired %q within the deadline", key)
}

// TestLock_HeldLockRefusesOthers asserts the direct form: while one holder has
// the key, another provider's acquisition fails rather than succeeding.
//
// It also pins the fail-fast semantic itself — the attempt returns immediately
// instead of waiting for the holder.
func TestLock_HeldLockRefusesOthers(t *testing.T) {
	t.Parallel()

	bucket := "refuse-" + uniqueSuffix()
	first, _ := locker(t, bucket)
	second, _ := locker(t, bucket)

	held, err := first.Lock(t.Context(), "resource")
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Release(context.Background()) })

	start := time.Now()
	_, err = second.Lock(t.Context(), "resource")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "a second holder must not acquire a held lock")
	require.Less(t, time.Since(start), lockTTL,
		"acquisition fails fast; it does not wait for the holder to finish")
}

// TestLock_SurvivesLongerThanTheAcquireTimeout asserts that the acquire timeout
// bounds waiting, not holding. Running the renewal on the context carrying that
// timeout dropped the lease the moment it elapsed — mid critical section.
func TestLock_SurvivesLongerThanTheAcquireTimeout(t *testing.T) {
	t.Parallel()

	const acquireTimeout = time.Second

	bucket := "acquiretimeout-" + uniqueSuffix()
	holderProv, _ := locker(t, bucket, locknats.WithAcquireTimeout(acquireTimeout))
	contender, _ := locker(t, bucket)

	dl := dlock.New(holderProv)

	var intruded bool
	err := dl.Synchronize(t.Context(), "resource", func(context.Context) error {
		deadline := time.Now().Add(4 * acquireTimeout)
		for time.Now().Before(deadline) {
			if lk, lockErr := contender.Lock(context.Background(), "resource"); lockErr == nil {
				intruded = true
				_ = lk.Release(context.Background())

				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}

		return nil
	})
	require.NoError(t, err)
	require.False(t, intruded, "the lock was released once the acquire timeout elapsed, while the holder still ran")
}

// TestLock_AbandonedLockExpires asserts the guarantee behind "a lock always has
// a TTL": a holder that dies without releasing still frees the key, because the
// server ages it out. This is the path no unit test reaches.
func TestLock_AbandonedLockExpires(t *testing.T) {
	t.Parallel()

	bucket := "abandon-" + uniqueSuffix()
	abandoned, conn := locker(t, bucket)
	survivor, _ := locker(t, bucket)

	held, err := abandoned.Lock(t.Context(), "resource")
	require.NoError(t, err)
	require.NotNil(t, held)

	// The contender must not get in while the lock is alive and renewing.
	_, err = survivor.Lock(t.Context(), "resource")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "precondition: the lock is held")

	// No Release, no Close: the connection simply goes away, as it would on a
	// SIGKILL. The lease is left for the server to reap.
	conn.Close()

	require.Eventually(t, func() bool {
		lk, lockErr := survivor.Lock(context.Background(), "resource")
		if lockErr != nil {
			return false
		}
		_ = lk.Release(context.Background())

		return true
	}, lockTTL+5*time.Second, 200*time.Millisecond,
		"an abandoned lock was never reaped — the key outlived its TTL")
}

// TestLock_FencingTokenAdvancesAcrossHolders asserts that each successive
// holder of a key sees a strictly higher fencing token. That is what lets a
// downstream store record the highest token it has accepted and reject a stale
// holder's late write.
func TestLock_FencingTokenAdvancesAcrossHolders(t *testing.T) {
	t.Parallel()

	bucket := "fencing-" + uniqueSuffix()
	prov, _ := locker(t, bucket)

	var tokens []uint64
	for range 4 {
		lk, err := prov.Lock(t.Context(), "resource")
		require.NoError(t, err)

		info, err := lk.GetLockInfo(t.Context())
		require.NoError(t, err)
		tokens = append(tokens, info.FencingToken)

		require.NoError(t, lk.Release(t.Context()))
	}

	for i, tok := range tokens {
		require.NotZero(t, tok, "holder %d reported no fencing token", i)
		if i == 0 {
			continue
		}
		require.Greater(t, tok, tokens[i-1],
			"holder %d opened at token %d, not above holder %d at %d", i, tok, i-1, tokens[i-1])
	}
}

// TestLock_ReleaseLeavesNoKey asserts that a released lock is gone immediately
// rather than lingering until its TTL, so the next contender does not wait.
func TestLock_ReleaseLeavesNoKey(t *testing.T) {
	t.Parallel()

	bucket := "release-" + uniqueSuffix()
	prov, _ := locker(t, bucket)

	lk, err := prov.Lock(t.Context(), "resource")
	require.NoError(t, err)
	require.NoError(t, lk.Release(t.Context()))

	_, err = prov.GetLockInfo(t.Context(), "resource")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "a released lock must not leave its key behind")

	// And the key is immediately available again, without waiting out the TTL.
	start := time.Now()
	next, err := prov.Lock(t.Context(), "resource")
	require.NoError(t, err)
	require.Less(t, time.Since(start), lockTTL, "re-acquiring after a release should not wait for expiry")
	require.NoError(t, next.Release(t.Context()))
}

// TestBucket_KeyTTLFollowsTheLockTTL asserts that the bucket the locks live in
// is configured with the lock TTL. The bucket's key TTL is what reaps an
// abandoned lock, so a bucket pinned to a different duration means a lock that
// either outlives its lease or expires before it.
func TestBucket_KeyTTLFollowsTheLockTTL(t *testing.T) {
	t.Parallel()

	nc := connect(t)
	ctx := t.Context()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	bucket := "buckettl-" + uniqueSuffix()
	const configured = 25 * time.Second

	_, err = locknats.New(ctx, nc, locknats.WithBucket(bucket), locknats.WithTTL(configured))
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteKeyValue(context.WithoutCancel(ctx), bucket) })

	kv, err := js.KeyValue(ctx, bucket)
	require.NoError(t, err)
	status, err := kv.Status(ctx)
	require.NoError(t, err)

	require.Equal(t, configured, status.TTL(),
		"the bucket key TTL must follow the configured lock TTL, not a constant")
}

// TestSynchronize_SerializesACounter is the end-to-end shape callers actually
// write: a read-modify-write through Synchronize from several processes. Under
// a working lock the counter equals the number of increments; under a broken
// one it does not.
func TestSynchronize_SerializesACounter(t *testing.T) {
	t.Parallel()

	const (
		workers    = 5
		increments = 4
	)

	bucket := "counter-" + uniqueSuffix()
	counterNc := connect(t)

	js, err := jetstream.New(counterNc)
	require.NoError(t, err)

	// The counter lives in its own bucket, deliberately without a TTL, so the
	// test measures the lock rather than the storage.
	counterBucket := "counterstate-" + uniqueSuffix()
	counter, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:  counterBucket,
		Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteKeyValue(context.WithoutCancel(t.Context()), counterBucket) })

	_, err = counter.Create(t.Context(), "n", []byte("0"))
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range workers {
		prov, _ := locker(t, bucket)
		dl := dlock.New(prov)

		wg.Go(func() {
			for range increments {
				synchronizeUntilDone(t, dl, "counter", func(ctx context.Context) error {
					entry, getErr := counter.Get(ctx, "n")
					if getErr != nil {
						return getErr
					}
					n, convErr := strconv.Atoi(string(entry.Value()))
					if convErr != nil {
						return convErr
					}

					// A deliberate gap between read and write: without mutual
					// exclusion this is where a lost update happens.
					time.Sleep(20 * time.Millisecond)

					_, putErr := counter.Put(ctx, "n", []byte(strconv.Itoa(n+1)))

					return putErr
				})
			}
		})
	}
	wg.Wait()

	entry, err := counter.Get(t.Context(), "n")
	require.NoError(t, err)

	got, err := strconv.Atoi(string(entry.Value()))
	require.NoError(t, err)
	require.Equal(t, workers*increments, got,
		"lost updates: the counter should equal every increment performed under the lock")
}
