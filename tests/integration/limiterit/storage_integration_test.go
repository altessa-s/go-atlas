// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters"
	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// TestStorage_Contract runs the sliding-window contract on every backend, the
// in-memory reference included, each scenario over its own namespace.
func TestStorage_Contract(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name       string
		sharedOnly bool
		run        func(t *testing.T, replica replicaFunc)
	}{
		{name: "ExactlyLimitPerWindow", run: exactlyLimitPerWindow},
		{name: "WindowSlidesPerRequest", run: windowSlidesPerRequest},
		{name: "ResetClearsTheWindow", run: resetClearsTheWindow},
		{name: "KeysAreIsolated", run: keysAreIsolated},
		{name: "ConcurrentCallersNeverExceedLimit", run: concurrentCallersNeverExceedLimit},
		{name: "ReplicasShareOneLimit", sharedOnly: true, run: replicasShareOneLimit},
	}

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			for _, sc := range scenarios {
				t.Run(sc.name, func(t *testing.T) {
					t.Parallel()
					if sc.sharedOnly && !b.shared {
						t.Skip("the in-memory provider is process-local; replicas do not share it")
					}
					sc.run(t, b.open(t))
				})
			}
		})
	}
}

// exactlyLimitPerWindow pins admission and the reported state call by call:
// exactly limit requests pass, Remaining counts down to zero, Reset is the
// second the oldest request leaves the window, and a rejection is the public
// sentinel by identity.
func exactlyLimitPerWindow(t *testing.T, replica replicaFunc) {
	const (
		limit  = 5
		period = 30 * time.Second
	)
	store := replica()
	ctx := t.Context()

	before := time.Now()
	first, err := store.Allow(ctx, "client-1", limit, period)
	after := time.Now()
	require.NoError(t, err)
	require.Equal(t, int64(limit-1), first.Remaining)
	// The window held no earlier request, so the admitted request is the
	// oldest one and its expiry is the reset time.
	lo, hi := resetBounds(before, after, period)
	require.GreaterOrEqual(t, first.Reset, lo, "first admission of an empty window")
	require.LessOrEqual(t, first.Reset, hi, "first admission of an empty window")

	for i := 2; i <= limit; i++ {
		info, err := store.Allow(ctx, "client-1", limit, period)
		require.NoError(t, err, "request %d of %d", i, limit)
		require.Equal(t, int64(limit-i), info.Remaining, "request %d", i)
		require.GreaterOrEqual(t, info.Reset, lo, "request %d: reset precedes the oldest request + period", i)
		require.LessOrEqual(t, info.Reset, hi, "request %d: reset follows the oldest request + period", i)
	}

	for range 3 {
		info, err := store.Allow(ctx, "client-1", limit, period)
		require.ErrorIs(t, err, storages.ErrLimitExceeded)
		require.ErrorIs(t, err, limiters.ErrLimitExceeded, "the storage sentinel is the public one")
		require.NotNil(t, info, "a rejection still reports the window state")
		require.Zero(t, info.Remaining)
		require.GreaterOrEqual(t, info.Reset, lo)
		require.LessOrEqual(t, info.Reset, hi)
	}
}

// windowSlidesPerRequest asserts a sliding window, not a fixed one: each
// request leaves the window period after it was made, so capacity comes back
// one request at a time, and rejected requests occupy no slot.
func windowSlidesPerRequest(t *testing.T, replica replicaFunc) {
	const (
		limit  = 3
		period = time.Second
		margin = 150 * time.Millisecond
	)
	store := replica()
	ctx := t.Context()
	allow := func() error {
		_, err := store.Allow(ctx, "slider", limit, period)
		return err
	}

	t0 := time.Now()
	require.NoError(t, allow(), "a")
	sleepUntil(t0.Add(period / 2))
	t1 := time.Now()
	require.NoError(t, allow(), "b")
	require.NoError(t, allow(), "c")
	require.ErrorIs(t, allow(), storages.ErrLimitExceeded, "window full")

	// a has left the window, b and c have not: exactly one slot is free.
	sleepUntil(t0.Add(period + margin))
	require.Less(t, time.Since(t1), period-margin/2, "the test host stalled; b and c may have expired too")
	require.NoError(t, allow(), "the slot of a")
	require.ErrorIs(t, allow(), storages.ErrLimitExceeded, "b and c are still inside the window")

	// b and c have left as well; the request admitted at t0+period is still in.
	sleepUntil(t1.Add(period + margin))
	require.NoError(t, allow(), "the slot of b")
	require.NoError(t, allow(), "the slot of c")
	require.ErrorIs(t, allow(), storages.ErrLimitExceeded, "full again")
}

// resetClearsTheWindow asserts Reset gives the key its whole limit back at
// once, and that resetting an unknown key is not an error.
func resetClearsTheWindow(t *testing.T, replica replicaFunc) {
	const limit = 4
	store := replica()
	ctx := t.Context()

	for range limit {
		_, err := store.Allow(ctx, "resettable", limit, time.Minute)
		require.NoError(t, err)
	}
	_, err := store.Allow(ctx, "resettable", limit, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)

	require.NoError(t, store.Reset(ctx, "resettable"))
	for i := range limit {
		info, err := store.Allow(ctx, "resettable", limit, time.Minute)
		require.NoError(t, err, "request %d after reset", i+1)
		require.Equal(t, int64(limit-i-1), info.Remaining)
	}
	_, err = store.Allow(ctx, "resettable", limit, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded, "the reset window fills up again")

	require.NoError(t, store.Reset(ctx, "never-used"), "resetting an unknown key")
}

// keysAreIsolated asserts that one exhausted key leaves another untouched,
// and that resetting one does not reset the other.
func keysAreIsolated(t *testing.T, replica replicaFunc) {
	const limit = 2
	store := replica()
	ctx := t.Context()

	for range limit {
		_, err := store.Allow(ctx, "tenant-a", limit, time.Minute)
		require.NoError(t, err)
	}
	_, err := store.Allow(ctx, "tenant-a", limit, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)

	info, err := store.Allow(ctx, "tenant-b", limit, time.Minute)
	require.NoError(t, err, "an exhausted key must not limit another")
	require.Equal(t, int64(limit-1), info.Remaining)

	require.NoError(t, store.Reset(ctx, "tenant-b"))
	_, err = store.Allow(ctx, "tenant-a", limit, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded, "resetting tenant-b must not reset tenant-a")
}

// concurrentCallersNeverExceedLimit is the property the limiter exists for:
// many goroutines racing one key admit no more than limit requests in total.
// Afterwards the window is topped up sequentially; it must take exactly the
// remainder, so a lost update cannot hide behind the concurrent phase.
func concurrentCallersNeverExceedLimit(t *testing.T, replica replicaFunc) {
	const (
		limit    = 40
		workers  = 32
		attempts = 8
	)
	store := replica()
	ctx := t.Context()

	res := hammer(workers, attempts, func(int) error {
		_, err := store.Allow(ctx, "hot-key", limit, time.Minute)
		return err
	})
	assertNeverExceeded(t, res, limit, func() error {
		_, err := store.Allow(ctx, "hot-key", limit, time.Minute)
		return err
	})
}

// replicasShareOneLimit runs the same race from two provider instances, each
// on its own connection: two replicas of a service limiting one client
// together, as the shared backends promise.
func replicasShareOneLimit(t *testing.T, replica replicaFunc) {
	const (
		limit    = 30
		workers  = 24
		attempts = 8
	)
	replicas := []storages.Storage{replica(), replica()}
	ctx := t.Context()

	res := hammer(workers, attempts, func(worker int) error {
		_, err := replicas[worker%len(replicas)].Allow(ctx, "shared-key", limit, time.Minute)
		return err
	})
	assertNeverExceeded(t, res, limit, func() error {
		_, err := replicas[1].Allow(ctx, "shared-key", limit, time.Minute)
		return err
	})
}

// assertNeverExceeded checks the outcome of a race on one key: never more
// than limit admitted, every call ends in a decision (the NATS provider
// absorbs CAS conflicts with retries), and sequential calls afterwards fill
// the window to exactly limit.
func assertNeverExceeded(t *testing.T, res *tally, limit int64, allow func() error) {
	t.Helper()

	admitted := res.admitted.Load()
	require.LessOrEqual(t, admitted, limit, "concurrent callers admitted more than the limit")
	require.Positive(t, admitted, "no concurrent caller was admitted")
	require.Empty(t, res.failures(), "calls failed without a decision")

	for {
		err := allow()
		if errors.Is(err, storages.ErrLimitExceeded) {
			break
		}
		require.NoError(t, err)
		admitted++
		require.LessOrEqual(t, admitted, limit, "the window admitted more than the limit")
	}
	require.Equal(t, limit, admitted, "the window does not hold exactly limit requests")
}

// TestStorage_MatchesMemory replays one call sequence on the in-memory
// reference and on each shared backend and compares the answers: the
// providers implement one algorithm and must agree on every admission,
// Remaining value and — within the second the calls straddle — Reset.
func TestStorage_MatchesMemory(t *testing.T) {
	t.Parallel()

	type step struct {
		key   string
		limit int64
		reset bool
	}
	script := []step{
		{key: "alpha", limit: 3}, {key: "alpha", limit: 3}, {key: "beta", limit: 2},
		{key: "alpha", limit: 3}, {key: "alpha", limit: 3}, {key: "beta", limit: 2},
		{key: "beta", limit: 2}, {key: "alpha", reset: true}, {key: "alpha", limit: 3},
		// A tighter limit on a key that already holds requests applies at once.
		{key: "alpha", limit: 1}, {key: "beta", limit: 5}, {key: "beta", limit: 5},
	}

	for _, b := range backends()[1:] {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			reference, store := openMemory(t)(), b.open(t)()
			ctx := t.Context()
			for i, s := range script {
				if s.reset {
					require.NoError(t, reference.Reset(ctx, s.key))
					require.NoError(t, store.Reset(ctx, s.key))
					continue
				}
				want, wantErr := reference.Allow(ctx, s.key, s.limit, time.Minute)
				got, gotErr := store.Allow(ctx, s.key, s.limit, time.Minute)

				require.Equal(t, wantErr, gotErr, "step %d (%s): admission differs from memory", i, s.key)
				require.Equal(t, want.Remaining, got.Remaining, "step %d (%s): Remaining differs from memory", i, s.key)
				require.InDelta(t, want.Reset, got.Reset, 1, "step %d (%s): Reset differs from memory", i, s.key)
			}
		})
	}
}
