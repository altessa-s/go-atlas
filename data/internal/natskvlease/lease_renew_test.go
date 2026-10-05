// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// errInjected stands in for a renewal failure that escapes KVOps' own retry:
// it is not network-classified, so it reaches the camping loop on the first
// attempt, exactly as a JetStream API error during a leader change does.
var errInjected = errors.New("injected renewal failure")

// faultyKV fails Get while its failure budget is positive. Acquiring a fresh
// key goes through Create, so only renewals (and releases) see the faults.
type faultyKV struct {
	jetstream.KeyValue

	failures atomic.Int64
	gets     atomic.Int64
	err      error

	// block makes a faulted Get hang until its context is done instead of
	// failing at once, the way a request to an unresponsive server does.
	block bool
}

func (f *faultyKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	f.gets.Add(1)
	if f.failures.Add(-1) >= 0 {
		if f.block {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, f.err
	}
	f.failures.Store(0)

	return f.KeyValue.Get(ctx, key)
}

// leaseEvents records the lease callbacks.
type leaseEvents struct {
	renewed  atomic.Int64
	lost     atomic.Int64
	released atomic.Int64

	mu     sync.Mutex
	lostAt time.Time
}

func (e *leaseEvents) callbacks() natskvlease.LeaseCallbacks {
	return natskvlease.LeaseCallbacks{
		OnRenewed: func() { e.renewed.Add(1) },
		OnLost: func() {
			e.mu.Lock()
			e.lostAt = time.Now()
			e.mu.Unlock()
			e.lost.Add(1)
		},
		OnReleased: func() { e.released.Add(1) },
	}
}

const (
	testLeaseTTL   = 3 * time.Second
	testLeaseKey   = "lease"
	testLeaseOwner = "owner-1"
)

// campSetup describes the lease a test camps on.
type campSetup struct {
	ctx      context.Context
	ttl      time.Duration
	ratio    float64
	injected error
	block    bool
}

// campLease starts a camping lease over a fault-injecting view of a fresh
// bucket and returns the view, the raw bucket, the lease and its events.
func campLease(t *testing.T, setup campSetup) (*faultyKV, jetstream.KeyValue, *natskvlease.Lease, *leaseEvents) {
	t.Helper()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ttl := cmp.Or(setup.ttl, testLeaseTTL)
	kv := testhelpers.CreateNATSKV(t, js, "leases", ttl)

	fkv := &faultyKV{KeyValue: kv, err: cmp.Or(setup.injected, errInjected), block: setup.block}
	events := &leaseEvents{}

	lease := natskvlease.NewLease(fkv, natskvlease.LeaseConfig{
		Key:        testLeaseKey,
		TTL:        ttl,
		RenewRatio: cmp.Or(setup.ratio, natskvlease.DefaultRenewRatio),
		Value:      []byte(testLeaseOwner),
		IsOwner:    func(v []byte) bool { return bytes.Equal(v, []byte(testLeaseOwner)) },
		Callbacks:  events.callbacks(),
	})

	acquired, err := lease.RunCamping(cmp.Or(setup.ctx, t.Context()), time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(lease.StopCamping)

	return fkv, kv, lease, events
}

// TestLease_RenewRetriesTransientFailure pins that a renewal failure while the
// lease still has life left is retried rather than reported as a lost lease.
// The loop used to give up on the first failed renew.
func TestLease_RenewRetriesTransientFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		failures int64
	}{
		{name: "one failure", failures: 1},
		{name: "two failures", failures: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fkv, kv, lease, events := campLease(t, campSetup{})

			fkv.failures.Store(tc.failures)

			require.Eventually(t, func() bool {
				return fkv.failures.Load() <= 0 && events.renewed.Load() > 0
			}, 2*testLeaseTTL, 20*time.Millisecond, "the lease was never renewed after the injected failures")

			require.Zero(t, events.lost.Load(), "a transient renewal failure was reported as a lost lease")
			require.True(t, lease.IsHeld())

			entry, err := kv.Get(t.Context(), testLeaseKey)
			require.NoError(t, err)
			require.Equal(t, testLeaseOwner, string(entry.Value()))
		})
	}
}

// TestLease_RenewGivesUpWhenLeaseCanExpire pins that renewal stops retrying,
// and reports the lease lost, once the lease could have expired on the server.
func TestLease_RenewGivesUpWhenLeaseCanExpire(t *testing.T) {
	t.Parallel()

	start := time.Now()
	fkv, _, lease, events := campLease(t, campSetup{})

	fkv.failures.Store(1 << 30)

	requireLostWithinTTL(t, start, lease, events)
	require.Greater(t, fkv.gets.Load(), int64(1), "the lease was reported lost without retrying the renewal")

	time.Sleep(testLeaseTTL / 3)
	require.Equal(t, int64(1), events.lost.Load(), "OnLost fired more than once")
}

// TestLease_RenewLostImmediatelyWhenTakenOver pins that losing the key to
// another owner is definitive: it is reported at once, not retried until the
// lease would have expired.
func TestLease_RenewLostImmediatelyWhenTakenOver(t *testing.T) {
	t.Parallel()

	_, kv, lease, events := campLease(t, campSetup{})

	_, err := kv.Put(t.Context(), testLeaseKey, []byte("owner-2"))
	require.NoError(t, err)
	takenAt := time.Now()

	require.Eventually(t, func() bool { return events.lost.Load() > 0 },
		testLeaseTTL, 10*time.Millisecond, "a lease taken over by another owner was not reported lost")

	events.mu.Lock()
	lostAt := events.lostAt
	events.mu.Unlock()

	interval := natskvlease.RenewInterval(testLeaseTTL, natskvlease.DefaultRenewRatio)
	require.Less(t, lostAt.Sub(takenAt), interval+interval/2, "a takeover was retried instead of being reported at once")
	require.False(t, lease.IsHeld())
}

// TestLease_RenewConnectionClosedIsDefinitive pins that a closed connection is
// not retried: it will not come back on its own.
func TestLease_RenewConnectionClosedIsDefinitive(t *testing.T) {
	t.Parallel()

	fkv, _, lease, events := campLease(t, campSetup{injected: nats.ErrConnectionClosed})

	fkv.failures.Store(1 << 30)

	require.Eventually(t, func() bool { return events.lost.Load() > 0 },
		testLeaseTTL, 10*time.Millisecond, "a renewal over a closed connection was not reported lost")
	require.Equal(t, int64(1), fkv.gets.Load(), "a closed connection was retried")
	require.False(t, lease.IsHeld())
}

// TestLease_CancelDuringRetryReleases pins that canceling the lease's context
// while a renewal is being retried releases the lease instead of reporting it
// lost or leaving it for the TTL to reap.
func TestLease_CancelDuringRetryReleases(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	fkv, kv, _, events := campLease(t, campSetup{ctx: ctx})

	fkv.failures.Store(1 << 30)
	require.Eventually(t, func() bool { return fkv.gets.Load() >= 2 },
		2*testLeaseTTL, 10*time.Millisecond, "the renewal was never retried")

	fkv.failures.Store(0)
	cancel()

	require.Eventually(t, func() bool { return events.released.Load() > 0 },
		testLeaseTTL, 10*time.Millisecond, "canceling the lease context during a retry did not release the lease")
	require.Zero(t, events.lost.Load())

	_, err := kv.Get(t.Context(), testLeaseKey)
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
}

// requireLostWithinTTL waits for the lease to be reported lost and checks that
// it happened before a lease written at start could have expired.
func requireLostWithinTTL(t *testing.T, start time.Time, lease *natskvlease.Lease, events *leaseEvents) {
	t.Helper()

	require.Eventually(t, func() bool { return events.lost.Load() > 0 },
		2*testLeaseTTL, 10*time.Millisecond, "a lease that can no longer be renewed was never reported lost")

	events.mu.Lock()
	lostAt := events.lostAt
	events.mu.Unlock()

	require.LessOrEqual(t, lostAt.Sub(start), testLeaseTTL, "the lease was still believed held after it could have expired")
	require.False(t, lease.IsHeld())
}

// TestLease_RenewBlockedAttempt pins how renewal treats a request that never
// gets an answer: one lost response must not use up the whole retry window,
// and an unresponsive server must not keep the holder believing it owns the
// lease past the point the server may have expired it.
func TestLease_RenewBlockedAttempt(t *testing.T) {
	t.Parallel()

	t.Run("recovers", func(t *testing.T) {
		t.Parallel()

		fkv, _, lease, events := campLease(t, campSetup{block: true})

		fkv.failures.Store(1)

		require.Eventually(t, func() bool {
			return fkv.failures.Load() <= 0 && events.renewed.Load() > 0
		}, 2*testLeaseTTL, 10*time.Millisecond, "the lease was never renewed after the unanswered request")

		require.Zero(t, events.lost.Load(), "one unanswered request was reported as a lost lease")
		require.True(t, lease.IsHeld())
	})

	t.Run("bounded by the deadline", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		fkv, _, lease, events := campLease(t, campSetup{block: true})

		fkv.failures.Store(1 << 30)

		requireLostWithinTTL(t, start, lease, events)
	})
}

// TestLease_HighRenewRatio pins that a renew ratio close to 1 still renews the
// lease before it can expire, and still gives up in time when it cannot.
func TestLease_HighRenewRatio(t *testing.T) {
	t.Parallel()

	t.Run("healthy", func(t *testing.T) {
		t.Parallel()

		_, kv, lease, events := campLease(t, campSetup{ratio: 0.95})

		first, err := kv.Get(t.Context(), testLeaseKey)
		require.NoError(t, err)

		time.Sleep(testLeaseTTL + testLeaseTTL/2)

		require.Zero(t, events.lost.Load(), "a healthy lease was reported lost")
		require.True(t, lease.IsHeld())

		entry, err := kv.Get(t.Context(), testLeaseKey)
		require.NoError(t, err)
		require.Greater(t, entry.Revision(), first.Revision(), "the lease was never renewed")
	})

	t.Run("one failure", func(t *testing.T) {
		t.Parallel()

		fkv, _, lease, events := campLease(t, campSetup{ratio: 0.95})

		fkv.failures.Store(1)

		require.Eventually(t, func() bool {
			return fkv.failures.Load() <= 0 && events.renewed.Load() > 0
		}, 2*testLeaseTTL, 10*time.Millisecond, "the lease was never renewed after the injected failure")

		require.Zero(t, events.lost.Load(), "one failure inside a short retry window was reported as a lost lease")
		require.True(t, lease.IsHeld())
	})

	t.Run("failing", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		fkv, _, lease, events := campLease(t, campSetup{ratio: 0.95})

		fkv.failures.Store(1 << 30)

		requireLostWithinTTL(t, start, lease, events)
	})
}

// TestLease_ShortTTLHighRatioRenews pins that a lease whose renewal window is
// shorter than the retry pacing still gets its renewal attempt: with a 100ms
// TTL at ratio 0.95 the window between the scheduled renewal and the deadline
// is about 10ms, and renewal must use it rather than give up unattempted.
func TestLease_ShortTTLHighRatioRenews(t *testing.T) {
	t.Parallel()

	const ttl = 100 * time.Millisecond

	_, _, lease, events := campLease(t, campSetup{ttl: ttl, ratio: 0.95})

	require.Eventually(t, func() bool { return events.renewed.Load() >= 3 },
		20*ttl, 5*time.Millisecond, "a healthy short lease was not renewed")
	require.Zero(t, events.lost.Load(), "a healthy short lease was reported lost")
	require.True(t, lease.IsHeld())
}
