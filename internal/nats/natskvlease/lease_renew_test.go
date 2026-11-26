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
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
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

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	ttl := cmp.Or(setup.ttl, testLeaseTTL)
	kv := natstest.CreateNATSKV(t, js, "leases", ttl)

	fkv := &faultyKV{KeyValue: kv, err: cmp.Or(setup.injected, errInjected), block: setup.block}
	events := &leaseEvents{}

	lease := natskvlease.NewLease(fkv, testLeaseKey, ttl, []byte(testLeaseOwner),
		natskvlease.WithLeaseRenewRatio(cmp.Or(setup.ratio, natskvlease.DefaultRenewRatio)),
		natskvlease.WithLeaseOwnerCheck(func(v []byte) bool { return bytes.Equal(v, []byte(testLeaseOwner)) }),
		natskvlease.WithLeaseCallbacks(events.callbacks()),
	)

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

// memKV is an in-memory [jetstream.KeyValue] holding the few operations a
// lease uses, so a lease can run inside a [synctest] bubble on virtual time.
// Every other method panics via the embedded nil interface.
type memKV struct {
	jetstream.KeyValue

	mu      sync.Mutex
	entries map[string]memEntry
	rev     uint64
	gets    atomic.Int64
}

type memEntry struct {
	jetstream.KeyValueEntry

	key   string
	value []byte
	rev   uint64
}

func (e memEntry) Key() string      { return e.key }
func (e memEntry) Value() []byte    { return e.value }
func (e memEntry) Revision() uint64 { return e.rev }

func (m *memKV) put(key string, value []byte) uint64 {
	m.rev++
	m.entries[key] = memEntry{key: key, value: bytes.Clone(value), rev: m.rev}
	return m.rev
}

func (m *memKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[key]; ok {
		return 0, jetstream.ErrKeyExists
	}
	return m.put(key, value), nil
}

func (m *memKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	m.gets.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return e, nil
}

func (m *memKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[key]; !ok || e.rev != revision {
		return 0, jetstream.ErrKeyExists
	}
	return m.put(key, value), nil
}

func (m *memKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

// ambiguousWriteKV can delay or lose the response after applying a write.
type ambiguousWriteKV struct {
	*memKV
	afterCreate    func(context.Context) error
	afterUpdate    func(context.Context) error
	checkedDeletes int
}

func (m *ambiguousWriteKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	rev, err := m.memKV.Create(ctx, key, value, opts...)
	if err == nil && m.afterCreate != nil {
		return rev, m.afterCreate(ctx)
	}
	return rev, err
}

func (m *ambiguousWriteKV) Update(ctx context.Context, key string, value []byte, rev uint64) (uint64, error) {
	rev, err := m.memKV.Update(ctx, key, value, rev)
	if err == nil && m.afterUpdate != nil {
		return rev, m.afterUpdate(ctx)
	}
	return rev, err
}

func (m *ambiguousWriteKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if len(opts) != 0 {
		m.checkedDeletes++
	}
	return m.memKV.Delete(ctx, key, opts...)
}

func TestLease_AmbiguousCreateIsReleased(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		kv := &ambiguousWriteKV{
			memKV: &memKV{entries: map[string]memEntry{}},
			afterCreate: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}
		lease := natskvlease.NewLease(kv, testLeaseKey, time.Second, []byte(testLeaseOwner))
		acquired, err := lease.RunCamping(t.Context(), time.Second)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, acquired)
		_, err = kv.Get(t.Context(), testLeaseKey)
		require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
		require.Equal(t, 1, kv.checkedDeletes)
	})
}

func TestLease_RejectsLateCreateAcknowledgment(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const ttl = time.Second
		ctx := t.Context()
		kv := &memKV{entries: map[string]memEntry{}}
		late := &ambiguousWriteKV{
			memKV: kv,
			afterCreate: func(context.Context) error {
				time.Sleep(ttl)
				return nil
			},
		}
		a := natskvlease.NewLease(late, testLeaseKey, ttl, []byte(testLeaseOwner))
		b := natskvlease.NewLease(kv, testLeaseKey, ttl, []byte("successor"))
		defer a.StopCamping()

		done := make(chan error, 1)
		go func() {
			_, err := a.RunCamping(ctx, 2*ttl)
			done <- err
		}()
		synctest.Wait()
		// The storage has expired the original entry before the reply reaches A.
		require.NoError(t, kv.Delete(ctx, testLeaseKey))
		acquired, err := b.Acquire(ctx)
		require.NoError(t, err)
		require.True(t, acquired)

		require.ErrorIs(t, <-done, natskvlease.ErrLeaseNotHeld)
		require.False(t, a.IsHeld())
		entry, err := kv.Get(ctx, testLeaseKey)
		require.NoError(t, err)
		require.Equal(t, "successor", string(entry.Value()))
	})
}

func TestLease_UnconfirmedRenewalStillReleases(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const ttl = time.Second
		kv := &ambiguousWriteKV{
			memKV: &memKV{entries: map[string]memEntry{}},
			afterUpdate: func(ctx context.Context) error {
				<-ctx.Done()
				return context.DeadlineExceeded
			},
		}
		lost := make(chan struct{})
		lease := natskvlease.NewLease(kv, testLeaseKey, ttl, []byte(testLeaseOwner),
			natskvlease.WithLeaseCallbacks(natskvlease.LeaseCallbacks{OnLost: func() { close(lost) }}))
		acquired, err := lease.RunCamping(t.Context(), time.Second)
		require.NoError(t, err)
		require.True(t, acquired)
		<-lost
		require.False(t, lease.IsHeld())

		require.NoError(t, lease.Release(t.Context()))
		_, err = kv.Get(t.Context(), testLeaseKey)
		require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
		require.Equal(t, 1, kv.checkedDeletes)
	})
}

// TestLease_ShortTTLHighRatioRenews pins that a lease whose renewal window is
// shorter than the retry pacing still gets its renewal attempt: with a 100ms
// TTL at ratio 0.95 the window between the scheduled renewal and the deadline
// is about 10ms, and renewal must use it rather than give up unattempted.
//
// It runs on virtual time against an in-memory bucket, so the result does not
// depend on how fast the host schedules the renewal or answers it: a renewal
// scheduled inside the window always completes there, and one scheduled at or
// past the deadline always reports the lease lost without a request.
func TestLease_ShortTTLHighRatioRenews(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const ttl = 100 * time.Millisecond

		kv := &memKV{entries: map[string]memEntry{}}
		events := &leaseEvents{}
		lease := natskvlease.NewLease(kv, testLeaseKey, ttl, []byte(testLeaseOwner),
			natskvlease.WithLeaseRenewRatio(0.95),
			natskvlease.WithLeaseOwnerCheck(func(v []byte) bool { return bytes.Equal(v, []byte(testLeaseOwner)) }),
			natskvlease.WithLeaseCallbacks(events.callbacks()),
		)

		acquired, err := lease.RunCamping(t.Context(), time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		time.Sleep(10 * ttl)
		synctest.Wait()

		require.Zero(t, events.lost.Load(), "a healthy short lease was reported lost")
		require.GreaterOrEqual(t, events.renewed.Load(), int64(3), "a healthy short lease was not renewed")
		require.Positive(t, kv.gets.Load())
		require.True(t, lease.IsHeld())

		lease.StopCamping()
		synctest.Wait()
	})
}
