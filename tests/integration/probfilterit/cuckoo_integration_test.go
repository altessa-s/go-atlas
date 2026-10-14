// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"

	cuckooredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
)

// TestCuckoo_InstancesShareAddsAndDeletes asserts that inserts and deletes by
// one process are what another sees, and that a delete reports whether it
// removed anything.
func TestCuckoo_InstancesShareAddsAndDeletes(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	b := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	ctx := t.Context()

	require.NoError(t, a.Add(ctx, "session:1"))
	require.NoError(t, a.AddBatch(ctx, slices.Values([]string{"session:2", "session:3"})))
	requirePresent(t, b, "session:1", "session:2", "session:3")

	deleted, err := b.Delete(ctx, "session:2")
	require.NoError(t, err)
	require.True(t, deleted)
	requireAbsent(t, a, "session:2")
	requirePresent(t, a, "session:1", "session:3")

	deleted, err = a.Delete(ctx, "session:2")
	require.NoError(t, err)
	require.False(t, deleted, "a value already deleted is not deleted again")
	deleted, err = a.Delete(ctx, "never-added")
	require.NoError(t, err)
	require.False(t, deleted)

	stats, err := b.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.ItemCount, "two inserted values remain")
	// RedisBloom rounds a reserved capacity of 1000 up to 512 buckets of two
	// fingerprints; CF.INFO "Size" is the footprint in bytes, not a capacity.
	require.Equal(t, int64(1024), stats.Capacity, "capacity counts fingerprint slots")
	require.Greater(t, stats.MemoryUsageBytes, stats.Capacity, "every slot takes a byte, plus the filter header")
	require.InDelta(t, 2.0/1024, stats.FillRatio, 1e-12)
}

// TestCuckoo_DuplicatesNeedOneDeleteEach pins the Cuckoo semantics a caller
// must know: inserting a value twice stores two fingerprints, so it stays
// present until it has been deleted twice.
func TestCuckoo_DuplicatesNeedOneDeleteEach(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	f := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	ctx := t.Context()

	require.NoError(t, f.Add(ctx, "dup"))
	require.NoError(t, f.Add(ctx, "dup"))

	deleted, err := f.Delete(ctx, "dup")
	require.NoError(t, err)
	require.True(t, deleted)
	requirePresent(t, f, "dup")

	deleted, err = f.Delete(ctx, "dup")
	require.NoError(t, err)
	require.True(t, deleted)
	requireAbsent(t, f, "dup")

	deleted, err = f.Delete(ctx, "dup")
	require.NoError(t, err)
	require.False(t, deleted)
}

// TestCuckoo_MissingFilter asserts that a filter nobody has written yet is
// recognized as "not created" from RedisBloom's real replies: a delete removes
// nothing, a lookup finds nothing and statistics report the configured
// capacity, none of them an error.
func TestCuckoo_MissingFilter(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	f := fx.cuckoo(t, cuckooredis.WithCapacity(2048))
	ctx := t.Context()

	deleted, err := f.Delete(ctx, "anything")
	require.NoError(t, err)
	require.False(t, deleted)
	requireAbsent(t, f, "anything")

	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2048), stats.Capacity)
	require.Zero(t, stats.ItemCount)
	require.Empty(t, fx.keys(t, fx.liveKey()), "a delete or lookup must not create the filter")
}

// TestCuckoo_BatchAndFalsePositiveRate inserts more values than one CF.INSERT
// carries and measures the false-positive rate over values never added.
// RedisBloom's Cuckoo filter keeps 8-bit fingerprints in buckets of two, so
// its rate is a few tenths of a percent at this load; the values are fixed,
// so the measurement is the same on every run.
func TestCuckoo_BatchAndFalsePositiveRate(t *testing.T) {
	t.Parallel()

	const items = 5000
	fx := newFixture(t)
	f := fx.cuckoo(t, cuckooredis.WithCapacity(2*items))

	vals := values("member", items)
	require.NoError(t, f.AddBatch(t.Context(), slices.Values(vals)))
	requirePresent(t, f, vals...)

	stats, err := f.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(items), stats.ItemCount)
	require.Equal(t, int64(16384), stats.Capacity, "a reserved capacity of 10000 is 8192 buckets of two slots")
	require.InDelta(t, float64(items)/16384, stats.FillRatio, 1e-12)

	rate := falsePositiveRate(t, f, values("stranger", 4*items))
	t.Logf("Cuckoo false-positive rate at %d/%d: %.4f", items, 2*items, rate)
	require.LessOrEqual(t, rate, 0.03, "false-positive rate far above an 8-bit fingerprint's")
}

// TestCuckoo_RebuildReplacesContents rebuilds from a snapshot larger than the
// configured capacity through both loading paths. The staging filter must be
// sized for it, values only the old contents held must be gone, and deletes
// must work on the rebuilt filter: the commit advanced the generation that
// fences deletes, and a delete reads it afresh.
func TestCuckoo_RebuildReplacesContents(t *testing.T) {
	t.Parallel()

	for _, counted := range []bool{true, false} {
		name := "collected"
		if counted {
			name = "streamed"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
			b := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
			ctx := t.Context()

			stale := values("stale", 20)
			require.NoError(t, a.AddBatch(ctx, slices.Values(stale)))
			deleted, err := a.Delete(ctx, stale[0])
			require.NoError(t, err)
			require.True(t, deleted, "a delete on the never-rebuilt filter")

			snapshot := values("fresh", 3000)
			require.NoError(t, a.Rebuild(ctx, openLoader(snapshot, counted)))

			requirePresent(t, b, snapshot...)
			requireAbsent(t, b, stale...)
			committed, err := b.RebuildCommitted(ctx)
			require.NoError(t, err)
			require.True(t, committed)
			fx.requireNoRebuildLeftovers(t)

			deleted, err = b.Delete(ctx, snapshot[0])
			require.NoError(t, err)
			require.True(t, deleted, "a delete on the rebuilt filter")
			requireAbsent(t, a, snapshot[0])
			requirePresent(t, a, snapshot[1:]...)
		})
	}
}

// TestCuckoo_InsertsDuringRebuild pins which concurrent inserts a Cuckoo
// rebuild keeps. Inserts through the rebuilding instance are journaled in
// process and replayed. Inserts by another process are not journaled in
// Redis — replaying one twice would store a duplicate fingerprint — so the
// commit drops them, as cuckoo.Filter.Rebuild documents.
func TestCuckoo_InsertsDuringRebuild(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	b := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	ctx := t.Context()

	loader := newGatedLoader(values("snapshot", 100), true)
	done := rebuildInBackground(t, a, loader)

	require.NoError(t, a.Add(ctx, "own-insert"))
	require.NoError(t, b.Add(ctx, "peer-insert"))
	requirePresent(t, b, "own-insert", "peer-insert")

	require.NoError(t, awaitRebuild(t, loader, done))
	requirePresent(t, b, loader.vals...)
	requirePresent(t, b, "own-insert")
	requireAbsent(t, b, "peer-insert")
	fx.requireNoRebuildLeftovers(t)
}

// TestCuckoo_SupersededRebuildDoesNotPublish expires a rebuild's lease while
// it runs and lets another instance take over. With no journal to drain, the
// commit script itself must notice the lease is no longer the stale
// rebuild's and refuse to rename.
func TestCuckoo_SupersededRebuildDoesNotPublish(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	b := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	ctx := t.Context()
	require.NoError(t, b.Add(ctx, "live"))

	stale := newGatedLoader(values("stale-snapshot", 50), true)
	staleDone := rebuildInBackground(t, a, stale)
	require.NoError(t, fx.admin.Del(ctx, fx.metaKey("rebuild-lease")).Err())

	// The lease lapsed, so the successor acquires it and publishes.
	successor := values("new-snapshot", 50)
	require.NoError(t, b.Rebuild(ctx, openLoader(successor, false)))

	err := awaitRebuild(t, stale, staleDone)
	require.ErrorIs(t, err, probfilter.ErrRebuildSuperseded)
	requirePresent(t, b, successor...)
	requireAbsent(t, b, "live")
	requireAbsent(t, b, values("stale-snapshot", 10)...)
	fx.requireNoRebuildLeftovers(t)

	// A rebuild that started after the successor published may publish again.
	require.NoError(t, a.Rebuild(ctx, openLoader(values("latest", 10), true)))
	requirePresent(t, b, values("latest", 10)...)
}

// TestCuckoo_ConcurrentRebuildIsRefused asserts the rebuild lease for the
// Cuckoo storage too: a second instance's rebuild fails fast while the first
// runs.
func TestCuckoo_ConcurrentRebuildIsRefused(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.cuckoo(t, cuckooredis.WithCapacity(1000))
	b := fx.cuckoo(t, cuckooredis.WithCapacity(1000))

	loader := newGatedLoader(values("first", 20), false)
	done := rebuildInBackground(t, a, loader)

	err := b.Rebuild(t.Context(), openLoader(values("second", 20), false))
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress)

	require.NoError(t, awaitRebuild(t, loader, done))
	requirePresent(t, b, values("first", 20)...)
}
