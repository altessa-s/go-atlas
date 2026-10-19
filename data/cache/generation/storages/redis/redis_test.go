// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/time/clock"
	"github.com/altessa-s/go-atlas/data/cache/generation"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/storagetest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	goredis "github.com/redis/go-redis/v9"
)

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

const staleness = time.Second

// replicas returns n stores sharing one Redis, each with its own client, and
// the fake clock they share. The Redis server clock is pinned to epoch.
func replicas(t testing.TB, n int, opts ...redis.Option) ([]*redis.Store, *miniredis.Miniredis, *clock.Fake) {
	t.Helper()
	client, mr := testhelpers.RedisClient(t)
	mr.SetTime(epoch)
	fake := clock.NewFake(epoch)
	opts = append([]redis.Option{redis.WithClock(fake), redis.WithMaxStaleness(staleness)}, opts...)
	stores := make([]*redis.Store, n)
	stores[0] = redis.New(client, opts...)
	for i := 1; i < n; i++ {
		other := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = other.Close() })
		stores[i] = redis.New(other, opts...)
	}
	return stores, mr, fake
}

func storedGen(t testing.TB, mr *miniredis.Miniredis, key string) uint64 {
	t.Helper()
	raw, err := mr.Get(key)
	require.NoError(t, err)
	v, err := strconv.ParseUint(raw, 10, 64)
	require.NoError(t, err)
	return v
}

// newBackend is the storagetest harness: n replicas sharing one miniredis,
// seeded from the server clock, with local staleness on the shared fake clock.
func newBackend(tb testing.TB, n int) storagetest.Backend {
	tb.Helper()
	stores, mr, fake := replicas(tb, n)
	gens := make([]generation.Store, n)
	for i, s := range stores {
		gens[i] = s
	}
	return storagetest.Backend{Stores: gens, SetSeedTime: mr.SetTime, Advance: fake.Advance}
}

func TestStore_Contract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newBackend, staleness)
}

func TestStore_CurrentSeedsMissingKeyFromServerClock(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 1)
	mr.SetTime(epoch.Add(time.Hour)) // the replica clock stays at epoch

	gen, err := stores[0].Current(t.Context(), "catalog")
	require.NoError(t, err)
	require.Equal(t, generation.Seed(epoch.Add(time.Hour)), gen, "seeded from the Redis server clock")
	require.Equal(t, gen, storedGen(t, mr, "generation:catalog"), "the seed is persisted for the other replicas")
}

func TestStore_KeyPrefix(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 1, redis.WithKeyPrefix("svc:gen"))

	_, err := stores[0].Bump(t.Context(), "catalog")
	require.NoError(t, err)
	require.True(t, mr.Exists("svc:gen:catalog"))
	require.False(t, mr.Exists("generation:catalog"))
}

func TestStore_OtherReplicaSeesBumpWithinTheBound(t *testing.T) {
	t.Parallel()
	stores, _, fake := replicas(t, 2)
	a, b := stores[0], stores[1]
	ctx := t.Context()

	g0, err := b.Current(ctx, "catalog") // b caches g0, checked at epoch
	require.NoError(t, err)

	g1, err := a.Bump(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, g1, g0)

	fake.Advance(staleness - time.Nanosecond)
	got, err := b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g0, got, "within the bound the local value may be reused")

	fake.Advance(time.Nanosecond)
	got, err = b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g1, got, "at the bound the bump is observed")
}

func TestStore_ZeroStalenessReadsRedisEveryTime(t *testing.T) {
	t.Parallel()
	stores, _, _ := replicas(t, 2, redis.WithMaxStaleness(0))
	a, b := stores[0], stores[1]
	ctx := t.Context()

	_, err := b.Current(ctx, "catalog")
	require.NoError(t, err)
	g1, err := a.Bump(ctx, "catalog")
	require.NoError(t, err)

	got, err := b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g1, got, "with no clock advance at all the bump is seen")
}

func TestStore_RestoredOlderGenerationFailsClosedAndHeals(t *testing.T) {
	t.Parallel()
	stores, mr, fake := replicas(t, 2)
	a, b := stores[0], stores[1]
	ctx := t.Context()

	g0, err := a.Current(ctx, "catalog")
	require.NoError(t, err)
	_, err = b.Current(ctx, "catalog")
	require.NoError(t, err)
	g1, err := a.Bump(ctx, "catalog")
	require.NoError(t, err)

	// A failover restores the older existing value.
	require.NoError(t, mr.Set("generation:catalog", strconv.FormatUint(g0, 10)))

	fake.Advance(staleness)
	_, err = a.Current(ctx, "catalog")
	require.ErrorIs(t, err, redis.ErrRegressed, "a replica that confirmed g1 must not trust g0")
	healed := storedGen(t, mr, "generation:catalog")
	require.Greater(t, healed, g1, "the key is healed above every generation the replica knew")

	got, err := a.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got, "reads resume from the healed value")

	got, err = b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got, "the other replica refreshes to the healed value")
}

func TestStore_RegressionIsDetectedOnlyAfterConfirmation(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 1, redis.WithMaxStaleness(0))
	ctx := t.Context()

	g1, err := stores[0].Bump(ctx, "catalog")
	require.NoError(t, err)
	require.NoError(t, mr.Set("generation:catalog", "5"))

	// With a frozen clock the read does not start strictly after the bump
	// was confirmed: it cannot prove a regression, and the local value wins.
	got, err := stores[0].Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g1, got, "the local value never goes backwards")
}

func TestStore_LostKeyIsReseededFromTheServerClock(t *testing.T) {
	t.Parallel()
	stores, mr, fake := replicas(t, 2)
	a, b := stores[0], stores[1]
	ctx := t.Context()

	_, err := a.Bump(ctx, "catalog")
	require.NoError(t, err)
	old, err := a.Bump(ctx, "catalog")
	require.NoError(t, err)

	mr.FlushAll()
	mr.SetTime(epoch.Add(time.Minute))

	// b has never seen the namespace: its first read after the loss reseeds
	// from the server clock, past every generation used before the loss.
	got, err := b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, got, old)

	mr.FlushAll()
	mr.SetTime(epoch.Add(2 * time.Minute))
	bumped, err := b.Bump(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, bumped, got, "a bump on a lost key is seeded from the server clock too")

	fake.Advance(staleness)
	got, err = a.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, bumped, got)
}

func TestStore_LostKeyReseededBelowAKnownGenerationHeals(t *testing.T) {
	t.Parallel()
	stores, mr, fake := replicas(t, 1)
	ctx := t.Context()

	_, err := stores[0].Bump(ctx, "catalog")
	require.NoError(t, err)
	known, err := stores[0].Bump(ctx, "catalog") // seed+2, ahead of the server clock
	require.NoError(t, err)

	mr.FlushAll() // the reseed (server time epoch) lands below known
	fake.Advance(staleness)
	_, err = stores[0].Current(ctx, "catalog")
	require.ErrorIs(t, err, redis.ErrRegressed)
	require.Greater(t, storedGen(t, mr, "generation:catalog"), known)

	got, err := stores[0].Current(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, got, known)
}

func TestStore_SkewedReplicaClocksNeverRegress(t *testing.T) {
	t.Parallel()
	client, mr := testhelpers.RedisClient(t)
	mr.SetTime(epoch)
	other := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = other.Close() })
	fast := redis.New(client, redis.WithClock(clock.NewFake(epoch.Add(time.Hour))), redis.WithMaxStaleness(0))
	slow := redis.New(other, redis.WithClock(clock.NewFake(epoch.Add(-time.Hour))), redis.WithMaxStaleness(0))
	ctx := t.Context()

	_, err := fast.Bump(ctx, "catalog")
	require.NoError(t, err)

	mr.FlushAll()
	seeded, err := slow.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, generation.Seed(epoch), seeded, "the slow replica seeds from the server, not its own clock")

	got, err := slow.Bump(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, got, seeded)

	// The stored value drops below what the slow replica has seen: a bump
	// still returns more than everything it observed.
	require.NoError(t, mr.Set("generation:catalog", "7"))
	next, err := slow.Bump(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, next, got)
	require.Equal(t, next, storedGen(t, mr, "generation:catalog"), "the bump changes the stored value, so keys change")
}

func TestStore_BumpAlwaysExceedsEverythingObserved(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 2)
	ctx := t.Context()

	seen := make([]uint64, len(stores)) // highest generation each replica observed
	for i := range 20 {
		r := i % len(stores)
		if i%3 == 0 {
			require.NoError(t, mr.Set("generation:catalog", "1")) // arbitrary rollbacks
		}
		gen, err := stores[r].Bump(ctx, "catalog")
		require.NoError(t, err)
		require.Greater(t, gen, seen[r], "a bump exceeds everything its replica has seen")
		seen[r] = gen
	}
}

func TestStore_UnreachableRedisNeverExtendsTheBound(t *testing.T) {
	t.Parallel()
	stores, mr, fake := replicas(t, 1)
	ctx := t.Context()

	g0, err := stores[0].Current(ctx, "catalog")
	require.NoError(t, err)

	mr.SetError("ERR down")
	fake.Advance(staleness - time.Nanosecond)
	got, err := stores[0].Current(ctx, "catalog")
	require.NoError(t, err, "a value within the bound needs no Redis call")
	require.Equal(t, g0, got)

	fake.Advance(time.Nanosecond)
	_, err = stores[0].Current(ctx, "catalog")
	require.Error(t, err, "a value at the bound must not be served without Redis")

	_, err = stores[0].Current(ctx, "never-seen")
	require.Error(t, err)

	mr.SetError("")
	got, err = stores[0].Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g0, got)
}

func TestStore_FailedBumpStaysPendingUntilPublished(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 2)
	a, b := stores[0], stores[1]
	ctx := t.Context()

	g0, err := a.Current(ctx, "catalog")
	require.NoError(t, err)

	mr.SetError("ERR down")
	_, err = a.Bump(ctx, "catalog")
	require.Error(t, err)

	// Without a clock advance the local value would still be within the
	// bound, but it predates a write: the replica must skip the cache.
	_, err = a.Current(ctx, "catalog")
	require.Error(t, err)

	mr.SetError("")
	published, err := a.Current(ctx, "catalog")
	require.NoError(t, err, "the pending bump is published on the next read")
	require.Greater(t, published, g0)
	require.Equal(t, published, storedGen(t, mr, "generation:catalog"))

	got, err := a.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, published, got, "published once, not on every read")

	other, err := b.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, published, other, "the other replica sees the published bump")
}

func TestStore_SuccessfulBumpClearsPending(t *testing.T) {
	t.Parallel()
	stores, mr, _ := replicas(t, 1)
	ctx := t.Context()

	mr.SetError("ERR down")
	_, err := stores[0].Bump(ctx, "catalog")
	require.Error(t, err)
	mr.SetError("")

	g, err := stores[0].Bump(ctx, "catalog")
	require.NoError(t, err)
	got, err := stores[0].Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, g, got, "the pending mark is cleared, no second increment")
}

// gateHook holds the first command sent through a client until released.
type gateHook struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (h *gateHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *gateHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *gateHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		first := false
		h.once.Do(func() { first = true })
		if first {
			close(h.entered)
			<-h.release
		}
		return next(ctx, cmd)
	}
}

func TestStore_BumpStartedBeforeAFailureDoesNotClearIt(t *testing.T) {
	t.Parallel()
	_, mr := testhelpers.RedisClient(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &gateHook{entered: make(chan struct{}), release: make(chan struct{})}
	client.AddHook(hook)
	s := redis.New(client, redis.WithClock(clock.NewFake(epoch)))
	ctx := t.Context()

	// Bump S starts first and is held before it reaches Redis.
	var wg sync.WaitGroup
	var early uint64
	var earlyErr error
	wg.Go(func() { early, earlyErr = s.Bump(ctx, "catalog") })
	<-hook.entered

	// Bump F starts later (its write committed later) and fails.
	mr.SetError("ERR down")
	_, err := s.Bump(ctx, "catalog")
	require.Error(t, err)
	mr.SetError("")

	close(hook.release)
	wg.Wait()
	require.NoError(t, earlyErr)

	// S's increment may predate F's write: F must still be published.
	published, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Greater(t, published, early, "the read publishes the failed bump instead of trusting the earlier one")
	require.Equal(t, published, storedGen(t, mr, "generation:catalog"))
}

func TestStore_ConcurrentUse(t *testing.T) {
	t.Parallel()
	stores, mr, fake := replicas(t, 2)
	ctx := t.Context()

	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, 2*n)
	for i := range n {
		wg.Go(func() {
			_, errs[2*i] = stores[i%2].Bump(ctx, "catalog")
		})
		wg.Go(func() {
			_, errs[2*i+1] = stores[i%2].Current(ctx, "catalog")
		})
	}
	wg.Wait()
	for i, err := range errs {
		if i%2 == 1 && errors.Is(err, redis.ErrRegressed) {
			// A read overlapped by a local bump that returns a higher value
			// fails closed by design (the other replica bumped meanwhile).
			continue
		}
		require.NoError(t, err)
	}

	// Settle healing bumps left pending, then every replica converges.
	for _, s := range stores {
		_, _ = s.Current(ctx, "catalog")
	}
	fake.Advance(staleness)
	stored := storedGen(t, mr, "generation:catalog")
	for _, s := range stores {
		got, err := s.Current(ctx, "catalog")
		require.NoError(t, err)
		require.Equal(t, stored, got)
	}
}

// hold is one armed gate: it stops one command either before it is sent
// (before) or after Redis executed it (after its reply arrived).
type hold struct {
	before  bool
	held    chan struct{}
	release chan struct{}
}

// queueGate applies armed holds, in order, to the next commands sent through
// a client; commands with no armed hold pass through.
type queueGate struct {
	mu    sync.Mutex
	queue []*hold
}

// arm queues a hold for the next unclaimed command and returns it.
func (g *queueGate) arm(before bool) *hold {
	h := &hold{before: before, held: make(chan struct{}), release: make(chan struct{})}
	g.mu.Lock()
	g.queue = append(g.queue, h)
	g.mu.Unlock()
	return h
}

func (g *queueGate) next() *hold {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.queue) == 0 {
		return nil
	}
	h := g.queue[0]
	g.queue = g.queue[1:]
	return h
}

func (g *queueGate) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (g *queueGate) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (g *queueGate) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h := g.next()
		if h == nil {
			return next(ctx, cmd)
		}
		if h.before {
			close(h.held)
			<-h.release
			return next(ctx, cmd)
		}
		err := next(ctx, cmd)
		close(h.held)
		<-h.release
		return err
	}
}

// gatedStore returns a store whose commands can be held, with both scripts
// already loaded so a held EVALSHA is never retried as EVAL. The Redis
// server clock is pinned to epoch.
func gatedStore(t testing.TB) (*redis.Store, *queueGate, *miniredis.Miniredis, *clock.Fake) {
	t.Helper()
	_, mr := testhelpers.RedisClient(t)
	mr.SetTime(epoch)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	gate := &queueGate{}
	client.AddHook(gate)
	fake := clock.NewFake(epoch)
	s := redis.New(client, redis.WithClock(fake), redis.WithMaxStaleness(0))
	_, err := s.Bump(t.Context(), "warmup")
	require.NoError(t, err)
	_, err = s.Current(t.Context(), "warmup")
	require.NoError(t, err)
	return s, gate, mr, fake
}

const gatedKey = "generation:catalog"

func setStored(t testing.TB, mr *miniredis.Miniredis, v uint64) {
	t.Helper()
	require.NoError(t, mr.Set(gatedKey, strconv.FormatUint(v, 10)))
}

// TestStore_ReadStartedBeforeBumpReturningAfterIt: the read executes before
// the write and the rollback, and its reply arrives after the bump completed.
func TestStore_ReadStartedBeforeBumpReturningAfterIt(t *testing.T) {
	t.Parallel()
	s, gate, mr, fake := gatedStore(t)
	ctx := t.Context()

	g0, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	high := g0 + 100
	setStored(t, mr, high) // another replica bumped

	fake.Advance(time.Millisecond)
	read := gate.arm(false)
	var wg sync.WaitGroup
	var lateErr error
	wg.Go(func() { _, lateErr = s.Current(ctx, "catalog") })
	<-read.held

	setStored(t, mr, g0) // the write commits, then a failover restores g0
	published, err := s.Bump(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, storedGen(t, mr, gatedKey), published, "a bump returns what Redis holds")
	require.Less(t, published, high)

	close(read.release)
	wg.Wait()
	require.ErrorIs(t, lateErr, redis.ErrRegressed)
	healed := storedGen(t, mr, gatedKey)
	require.Greater(t, healed, high, "healed above the pre-write generation")
	got, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got)
}

// TestStore_ReadStartedAfterBumpStartedReturningAfterIt is the round-3 case:
// the bump starts and pauses before its script, a read starts, executes
// before the rollback and returns after the bump completed.
func TestStore_ReadStartedAfterBumpStartedReturningAfterIt(t *testing.T) {
	t.Parallel()
	s, gate, mr, fake := gatedStore(t)
	ctx := t.Context()

	g0, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	high := g0 + 100

	bump := gate.arm(true)
	var wg sync.WaitGroup
	var published uint64
	var bumpErr error
	bumpDone := make(chan struct{})
	wg.Go(func() {
		defer close(bumpDone)
		published, bumpErr = s.Bump(ctx, "catalog")
	})
	<-bump.held

	fake.Advance(time.Millisecond) // the read starts after the bump started
	setStored(t, mr, high)
	read := gate.arm(false)
	var lateErr error
	wg.Go(func() { _, lateErr = s.Current(ctx, "catalog") })
	<-read.held

	setStored(t, mr, g0) // rollback below high; the server clock is below high too
	close(bump.release)
	<-bumpDone // the bump completes before the delayed reply arrives
	close(read.release)
	wg.Wait()

	require.NoError(t, bumpErr)
	require.Less(t, published, high, "the bump could not know high")
	require.ErrorIs(t, lateErr, redis.ErrRegressed, "an overlapped read above the bump is never installed")
	healed := storedGen(t, mr, gatedKey)
	require.Greater(t, healed, high)
	got, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got)
	require.NotEqual(t, high, got)
}

// TestStore_ReadOverlappingTwoConcurrentBumps: both bumps pause before their
// scripts while a read of a pre-write value is held.
func TestStore_ReadOverlappingTwoConcurrentBumps(t *testing.T) {
	t.Parallel()
	s, gate, mr, _ := gatedStore(t)
	ctx := t.Context()

	g0, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	high := g0 + 100

	var wg sync.WaitGroup
	results := make([]uint64, 2)
	errs := make([]error, 2)
	bumps := make([]*hold, 2)
	done := make([]chan struct{}, 2)
	for i := range 2 {
		bumps[i] = gate.arm(true)
		done[i] = make(chan struct{})
		wg.Go(func() {
			defer close(done[i])
			results[i], errs[i] = s.Bump(ctx, "catalog")
		})
		<-bumps[i].held
	}

	setStored(t, mr, high)
	read := gate.arm(false)
	var lateErr error
	wg.Go(func() { _, lateErr = s.Current(ctx, "catalog") })
	<-read.held

	setStored(t, mr, g0)
	for i := range 2 {
		close(bumps[i].release)
		<-done[i]
		require.Equal(t, storedGen(t, mr, gatedKey), results[i], "each bump returns what Redis holds")
		require.Less(t, results[i], high)
	}
	close(read.release)
	wg.Wait()

	for i := range 2 {
		require.NoError(t, errs[i])
	}
	require.ErrorIs(t, lateErr, redis.ErrRegressed)
	healed := storedGen(t, mr, gatedKey)
	require.Greater(t, healed, high)
	require.Greater(t, healed, max(results[0], results[1]))
	got, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got, "high is never installed")
}

// TestStore_ReadRaisingTheMaximumDuringBumpRepublishes: the bump reply is
// held while a read observes a higher generation.
func TestStore_ReadRaisingTheMaximumDuringBumpRepublishes(t *testing.T) {
	t.Parallel()
	s, gate, mr, fake := gatedStore(t)
	ctx := t.Context()

	g0, err := s.Current(ctx, "catalog")
	require.NoError(t, err)

	fake.Advance(time.Millisecond)
	bump := gate.arm(false)
	var wg sync.WaitGroup
	var bumped uint64
	var bumpErr error
	wg.Go(func() { bumped, bumpErr = s.Bump(ctx, "catalog") })
	<-bump.held

	high := g0 + 100
	setStored(t, mr, high)
	_, err = s.Current(ctx, "catalog")
	require.ErrorIs(t, err, redis.ErrRegressed, "a read overlapping a bump never installs a higher value")

	close(bump.release)
	wg.Wait()
	require.NoError(t, bumpErr)
	require.Greater(t, bumped, high, "the bump exceeds what the replica observed when it completed")
	require.Equal(t, storedGen(t, mr, gatedKey), bumped, "and it is the value Redis holds")

	got, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, bumped, got)
}

// TestStore_NonOverlappingReadsInstallNormally: with no bump on this replica,
// concurrent reads follow another replica's bumps without false regressions.
func TestStore_NonOverlappingReadsInstallNormally(t *testing.T) {
	t.Parallel()
	stores, _, _ := replicas(t, 2, redis.WithMaxStaleness(0))
	writer, reader := stores[0], stores[1]
	ctx := t.Context()

	var last uint64
	for range 5 {
		gen, err := writer.Bump(ctx, "catalog")
		require.NoError(t, err)
		var wg sync.WaitGroup
		errs := make([]error, 8)
		gens := make([]uint64, 8)
		for i := range 8 {
			wg.Go(func() { gens[i], errs[i] = reader.Current(ctx, "catalog") })
		}
		wg.Wait()
		for i := range 8 {
			require.NoError(t, errs[i])
			require.Equal(t, gen, gens[i])
		}
		require.Greater(t, gen, last)
		last = gen
	}
}

// TestStore_ReadSpanningAFailedBumpIsNotInstalled: a pre-write read whose
// reply arrives after a failed bump never becomes the current generation.
func TestStore_ReadSpanningAFailedBumpIsNotInstalled(t *testing.T) {
	t.Parallel()
	s, gate, mr, _ := gatedStore(t)
	ctx := t.Context()

	g0, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	high := g0 + 100
	setStored(t, mr, high)

	read := gate.arm(false)
	var wg sync.WaitGroup
	var lateErr error
	wg.Go(func() { _, lateErr = s.Current(ctx, "catalog") })
	<-read.held

	mr.SetError("ERR down")
	_, err = s.Bump(ctx, "catalog")
	require.Error(t, err)
	mr.SetError("")
	setStored(t, mr, g0) // rollback

	close(read.release)
	wg.Wait()
	require.ErrorIs(t, lateErr, redis.ErrRegressed)
	healed := storedGen(t, mr, gatedKey)
	require.Greater(t, healed, high, "the healing bump publishes the failed one above high")
	got, err := s.Current(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, healed, got)
}
