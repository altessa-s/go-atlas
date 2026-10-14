// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"

	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
)

// TestBloom_InstancesShareOneFilter asserts that two processes opening the
// same filter see each other's inserts, single and batched, and that the
// filter the first write reserves carries the configured capacity.
func TestBloom_InstancesShareOneFilter(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	ctx := t.Context()

	require.NoError(t, a.Add(ctx, "user:1"))
	requirePresent(t, b, "user:1")
	requireAbsent(t, b, "user:2", "user:3")

	require.NoError(t, b.AddBatch(ctx, slices.Values([]string{"user:2", "user:3", "user:4"})))
	requirePresent(t, a, "user:1", "user:2", "user:3", "user:4")
	require.NoError(t, a.Add(ctx, "user:1"), "re-adding a present value")

	stats, err := a.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1000), stats.Capacity, "the filter must be reserved with the configured capacity")
	require.Equal(t, int64(4), stats.ItemCount)
	require.Equal(t, "redis", stats.StorageType)
	require.Positive(t, stats.MemoryUsageBytes)
}

// TestBloom_StatsOfMissingFilter asserts that BF.INFO on a filter nobody has
// written yet is recognized as "not created" on a real server, not reported
// as a failure.
func TestBloom_StatsOfMissingFilter(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	f := fx.bloom(t, bloomredis.WithExpectedItems(2500))

	stats, err := f.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2500), stats.Capacity)
	require.Zero(t, stats.ItemCount)

	committed, err := f.RebuildCommitted(t.Context())
	require.NoError(t, err)
	require.False(t, committed)
}

// TestBloom_BatchLargerThanOneCommand inserts more values than one BF.INSERT
// carries, so AddBatch must split them across commands and lose none.
func TestBloom_BatchLargerThanOneCommand(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	f := fx.bloom(t, bloomredis.WithExpectedItems(5000), bloomredis.WithFalsePositiveRate(0.001))
	vals := values("batch", 2500)

	require.NoError(t, f.AddBatch(t.Context(), slices.Values(vals)))
	requirePresent(t, f, vals...)

	stats, err := f.Stats(t.Context())
	require.NoError(t, err)
	// BF.INSERT does not count a value whose bits were all set already, so a
	// handful of in-batch collisions may go uncounted.
	require.InDelta(t, len(vals), stats.ItemCount, float64(len(vals))/100)
}

// TestBloom_FalsePositiveRate fills a filter to its expected size and
// measures its false-positive rate over values never added. The values are
// fixed, so the measurement is the same on every run.
func TestBloom_FalsePositiveRate(t *testing.T) {
	t.Parallel()

	const (
		items  = 5000
		target = 0.01
	)
	fx := newFixture(t)
	f := fx.bloom(t, bloomredis.WithExpectedItems(items), bloomredis.WithFalsePositiveRate(target))

	vals := values("member", items)
	require.NoError(t, f.AddBatch(t.Context(), slices.Values(vals)))
	requirePresent(t, f, vals...)

	rate := falsePositiveRate(t, f, values("stranger", 4*items))
	t.Logf("Bloom false-positive rate at capacity: %.4f (target %.4f)", rate, target)
	require.LessOrEqual(t, rate, 2*target, "false-positive rate far above the configured target")
}

// TestBloom_RebuildReplacesContents rebuilds a shared filter from a new
// snapshot through both loading paths — a counted loader streamed into a
// sized staging filter, and an uncounted one collected first — and asserts the
// swap: the new snapshot is in, values only the old contents held are gone,
// the other instance sees the result and its commit, and nothing of the
// rebuild is left behind.
func TestBloom_RebuildReplacesContents(t *testing.T) {
	t.Parallel()

	for _, counted := range []bool{true, false} {
		name := "collected"
		if counted {
			name = "streamed"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
			b := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
			ctx := t.Context()

			stale := values("stale", 20)
			require.NoError(t, a.AddBatch(ctx, slices.Values(stale)))

			// More than the configured size: either path sizes its staging
			// filter for the snapshot.
			snapshot := values("fresh", 3000)
			require.NoError(t, a.Rebuild(ctx, openLoader(snapshot, counted)))

			requirePresent(t, b, snapshot...)
			requireAbsent(t, b, stale...)
			require.False(t, a.LastRebuild().IsZero())
			require.True(t, b.LastRebuild().IsZero(), "LastRebuild is per process")
			for _, f := range []probfilter.RebuildCommitReporter{a, b} {
				committed, err := f.RebuildCommitted(ctx)
				require.NoError(t, err)
				require.True(t, committed, "every instance must see the committed rebuild")
			}
			stats, err := b.Stats(ctx)
			require.NoError(t, err)
			require.GreaterOrEqual(t, stats.Capacity, int64(len(snapshot)), "the staging filter was not sized for the snapshot")
			fx.requireNoRebuildLeftovers(t)

			// The rebuilt filter is the live one: inserts land in it.
			require.NoError(t, b.Add(ctx, "after-rebuild"))
			requirePresent(t, a, "after-rebuild")
		})
	}
}

// TestBloom_FailedRebuildKeepsContents asserts that a rebuild whose source
// fails midway publishes nothing: the live filter, its commit state and its
// rebuild lease are as before, and its staging key is gone.
func TestBloom_FailedRebuildKeepsContents(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	ctx := t.Context()

	live := values("live", 10)
	require.NoError(t, a.AddBatch(ctx, slices.Values(live)))

	partial := values("partial", 1500)
	loader := openLoader(partial, true)
	loader.failAfter = 1200 // past the first staging batch, so the staging key holds values
	err := a.Rebuild(ctx, loader)
	require.ErrorIs(t, err, errSourceFailed)

	requirePresent(t, b, live...)
	requireAbsent(t, b, partial[:10]...)
	require.True(t, a.LastRebuild().IsZero(), "a failed rebuild must not count as one")
	committed, err := b.RebuildCommitted(ctx)
	require.NoError(t, err)
	require.False(t, committed)
	fx.requireNoRebuildLeftovers(t)

	// The lease was released: another instance can rebuild at once.
	require.NoError(t, b.Rebuild(ctx, openLoader(values("recovered", 5), true)))
	requirePresent(t, a, values("recovered", 5)...)
}

// TestBloom_InsertsDuringRebuildSurvive is the journal's reason to exist: a
// rebuild renames its staging filter over the live key, so every insert made
// while it ran — by the rebuilding instance, by another instance, and by one
// that keeps writing through the commit — must be carried over.
//
// The peer's burst is larger than one journal drain batch, and the stream of
// inserts that runs through the commit lands some values in the drain and the
// rest in the commit script's replay or in the new live filter, whichever the
// interleaving produces. An insert acknowledged at any point must be present.
func TestBloom_InsertsDuringRebuildSurvive(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(5000), bloomredis.WithFalsePositiveRate(0.001))
	b := fx.bloom(t, bloomredis.WithExpectedItems(5000), bloomredis.WithFalsePositiveRate(0.001))
	c := fx.bloom(t, bloomredis.WithExpectedItems(5000), bloomredis.WithFalsePositiveRate(0.001))
	ctx := t.Context()

	require.NoError(t, a.Add(ctx, "before-rebuild"))

	loader := newGatedLoader(values("snapshot", 500), true)
	done := rebuildInBackground(t, a, loader)

	require.NoError(t, a.Add(ctx, "own-insert"), "an insert by the rebuilding instance")
	peerBurst := values("peer", 1200)
	require.NoError(t, b.AddBatch(ctx, slices.Values(peerBurst)), "a burst by another instance")
	journaled, err := fx.admin.LLen(ctx, fx.metaKey("rebuild-journal")).Result()
	require.NoError(t, err)
	require.GreaterOrEqual(t, journaled, int64(len(peerBurst)+1), "inserts under a rebuild lease must be journaled in Redis")

	// A writer that keeps going until the rebuild has returned.
	var (
		acked []string
		stop  = make(chan struct{})
		wg    sync.WaitGroup
	)
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			v := fmt.Sprintf("stream-%06d", i)
			if err := c.Add(ctx, v); err == nil {
				acked = append(acked, v)
			}
		}
	})

	time.Sleep(20 * time.Millisecond) // let the writer get going before the commit
	require.NoError(t, awaitRebuild(t, loader, done))
	close(stop)
	wg.Wait()

	requirePresent(t, a, loader.vals...)
	requirePresent(t, a, "own-insert")
	requirePresent(t, a, peerBurst...)
	requirePresent(t, a, acked...)
	requireAbsent(t, a, "before-rebuild")
	t.Logf("%d inserts acknowledged by a writer running through the commit", len(acked))
	fx.requireNoRebuildLeftovers(t)
}

// TestBloom_ConcurrentRebuildIsRefused asserts the rebuild lease across
// processes: while one instance rebuilds, another's rebuild fails fast with
// ErrRebuildInProgress instead of racing it, and succeeds once it is done.
func TestBloom_ConcurrentRebuildIsRefused(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1000))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1000))
	ctx := t.Context()

	loader := newGatedLoader(values("first", 50), true)
	done := rebuildInBackground(t, a, loader)

	start := time.Now()
	err := b.Rebuild(ctx, openLoader(values("second", 50), true))
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress)
	require.Less(t, time.Since(start), 5*time.Second, "a refused rebuild must not wait for the holder")

	// The running rebuild's staging key expires on its own if the process
	// dies; the refused one created none.
	staging := fx.keys(t, fx.metaKey("staging")+"*")
	require.Len(t, staging, 1, "exactly the running rebuild's staging key")
	ttl, err := fx.admin.PTTL(ctx, staging[0]).Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "a staging key must carry an expiry")
	require.LessOrEqual(t, ttl, time.Hour)

	require.NoError(t, awaitRebuild(t, loader, done))
	requirePresent(t, b, values("first", 50)...)
	ttl, err = fx.admin.PTTL(ctx, fx.liveKey()).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), ttl, "the promoted filter must not keep the staging expiry")

	require.NoError(t, b.Rebuild(ctx, openLoader(values("second", 50), true)))
	requirePresent(t, a, values("second", 50)...)
	requireAbsent(t, a, values("first", 5)...)
}

// TestBloom_SupersededRebuildDoesNotPublish expires a rebuild's lease while it
// runs, as a stalled process would lose it, and lets another instance take
// over. The stale rebuild must refuse to publish — its journal drain sees the
// successor's lease — while the successor publishes its own snapshot together
// with the inserts journaled under its lease.
func TestBloom_SupersededRebuildDoesNotPublish(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	c := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	ctx := t.Context()
	require.NoError(t, c.Add(ctx, "live"))

	stale := newGatedLoader(values("stale-snapshot", 50), true)
	staleDone := rebuildInBackground(t, a, stale)

	// The stale holder's lease runs out.
	require.NoError(t, fx.admin.Del(ctx, fx.metaKey("rebuild-lease")).Err())

	successor := newGatedLoader(values("new-snapshot", 50), true)
	successorDone := rebuildInBackground(t, b, successor)
	require.NoError(t, c.Add(ctx, "under-successor"), "an insert journaled under the successor's lease")

	err := awaitRebuild(t, stale, staleDone)
	require.ErrorIs(t, err, probfilter.ErrRebuildSuperseded)
	requirePresent(t, c, "live")
	requireAbsent(t, c, values("stale-snapshot", 10)...)
	require.True(t, a.LastRebuild().IsZero())

	require.NoError(t, awaitRebuild(t, successor, successorDone))
	requirePresent(t, c, successor.vals...)
	requirePresent(t, c, "under-successor")
	requireAbsent(t, c, values("stale-snapshot", 10)...)
	fx.requireNoRebuildLeftovers(t)
}

// TestBloom_RecreatedFilterIsNotRebuilt deletes the live key after a
// committed rebuild, as an eviction or an operator would. The commit report
// must drop to false, and the next insert must recreate the filter with its
// configured parameters — its NOCREATE insert fails with RedisBloom's
// "not found", which the storage has to recognize — without restoring the
// commit report.
func TestBloom_RecreatedFilterIsNotRebuilt(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1500))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1500))
	ctx := t.Context()

	require.NoError(t, a.Rebuild(ctx, openLoader(values("snapshot", 10), true)))
	require.NoError(t, b.Add(ctx, "reserved-once"), "b reserves the filter once, then inserts without creating")

	require.NoError(t, fx.admin.Del(ctx, fx.liveKey()).Err())
	committed, err := a.RebuildCommitted(ctx)
	require.NoError(t, err)
	require.False(t, committed, "a deleted filter cannot hold a committed rebuild")

	require.NoError(t, b.Add(ctx, "after-delete"), "an insert into a vanished filter must recreate it")
	requirePresent(t, a, "after-delete")
	committed, err = a.RebuildCommitted(ctx)
	require.NoError(t, err)
	require.False(t, committed, "a recreated filter does not hold a committed rebuild")

	stats, err := a.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1500), stats.Capacity, "the filter was recreated without its configured capacity")
}

// TestBloom_CloseAbortsRunningRebuild closes a filter while its rebuild holds
// the lease. The rebuild must return ErrFilterClosed without publishing and
// leave neither its staging key nor its lease behind, so another process can
// rebuild at once.
func TestBloom_CloseAbortsRunningRebuild(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	b := fx.bloom(t, bloomredis.WithExpectedItems(1000), bloomredis.WithFalsePositiveRate(0.001))
	ctx := t.Context()
	require.NoError(t, b.Add(ctx, "live"))

	loader := newGatedLoader(values("abandoned", 50), true)
	done := rebuildInBackground(t, a, loader)

	require.NoError(t, a.Close(ctx))
	select {
	case err := <-done:
		require.ErrorIs(t, err, probfilter.ErrFilterClosed)
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not interrupt the rebuild")
	}

	requirePresent(t, b, "live")
	requireAbsent(t, b, values("abandoned", 10)...)
	fx.requireNoRebuildLeftovers(t)
	require.NoError(t, b.Rebuild(ctx, openLoader(values("next", 10), true)), "the lease must be free")
}
