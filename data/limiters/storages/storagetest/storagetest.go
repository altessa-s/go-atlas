// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// contracts lists the multi-key contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, storages.MultiStorage)
}{
	{"AllowAllTakesEverySlot", AllowAllTakesEverySlot},
	{"AllowAllRefusalTakesNothing", AllowAllRefusalTakesNothing},
	{"AllowAllReportsFirstRefusal", AllowAllReportsFirstRefusal},
	{"AllowAllRejectsDuplicateKeys", AllowAllRejectsDuplicateKeys},
	{"AllowAllConcurrentAllOrNothing", AllowAllConcurrentAllOrNothing},
}

// period is long enough that no window expires while a contract runs.
const period = time.Minute

// The keys share the hash tag {t}, so a Redis Cluster backend places every
// key of one call in one slot.
const (
	keyGlobal = "{t}:global"
	keyHost   = "{t}:host-"
)

func host(i int) string { return keyHost + strconv.Itoa(i) }

// Run executes every contract as a parallel subtest on a fresh store from
// newStore, which must return an empty store isolated from every other store
// it returns. The names in skip are reported as skipped.
func Run(t *testing.T, newStore func(testing.TB) storages.MultiStorage, skip ...string) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if slices.Contains(skip, c.name) {
				t.Skip("not supported by this backend")
			}
			c.check(t, newStore(t))
		})
	}
}

// probeLimit is the limit of the probe in [taken]: far above every limit a
// contract sets, so the probe always admits.
const probeLimit = 1 << 20

// taken returns how many slots of key are in its window, read through one
// probing Allow call that takes a slot itself.
func taken(tb testing.TB, s storages.Storage, key string) int64 {
	tb.Helper()
	info, err := s.Allow(tb.Context(), key, probeLimit, period)
	require.NoError(tb, err, "probe of %s", key)
	return probeLimit - 1 - info.Remaining
}

// AllowAllTakesEverySlot admits a call on two keys, takes a slot on each and
// reports the state after the admission per key, in input order.
//
//nolint:mnd // Fixed limits and counts describe the storage contract.
func AllowAllTakesEverySlot(t *testing.T, s storages.MultiStorage) {
	t.Helper()
	infos, err := s.AllowAll(t.Context(), []storages.Limit{
		{Key: host(1), Limit: 2, Period: period},
		{Key: keyGlobal, Limit: 3, Period: period},
	})
	require.NoError(t, err)
	require.Len(t, infos, 2)
	require.Equal(t, int64(1), infos[0].Remaining)
	require.Equal(t, int64(2), infos[1].Remaining)
	require.Positive(t, infos[0].Reset)
	require.Positive(t, infos[1].Reset)

	require.Equal(t, int64(1), taken(t, s, host(1)))
	require.Equal(t, int64(1), taken(t, s, keyGlobal))
}

// AllowAllRefusalTakesNothing is the slot-burning regression: a host key
// that admits must keep its slot when the global key of the same call
// refuses. Two sequential Allow calls take the host slot and then refuse.
//
//nolint:mnd // Fixed limits and counts describe the storage contract.
func AllowAllRefusalTakesNothing(t *testing.T, s storages.MultiStorage) {
	t.Helper()
	ctx := t.Context()
	_, err := s.Allow(ctx, keyGlobal, 1, period)
	require.NoError(t, err)

	limits := []storages.Limit{
		{Key: host(1), Limit: 3, Period: period},
		{Key: keyGlobal, Limit: 1, Period: period},
	}
	for range 5 {
		infos, err := s.AllowAll(ctx, limits)
		require.ErrorIs(t, err, storages.ErrLimitExceeded)
		keyErr, ok := errors.AsType[*storages.KeyError](err)
		require.True(t, ok, "%T", err)
		require.Equal(t, 1, keyErr.Index)
		require.Equal(t, keyGlobal, keyErr.Key)
		require.Len(t, infos, 2)
		require.Equal(t, int64(3), infos[0].Remaining, "the admitting key reports its free slots, none taken")
		require.Equal(t, int64(0), infos[1].Remaining)
	}

	require.Equal(t, int64(0), taken(t, s, host(1)), "refused calls must not take a host slot")
}

// AllowAllReportsFirstRefusal refuses on the lowest refusing index and
// reports every key: Remaining is 0 exactly for the refusing keys.
//
//nolint:mnd // Fixed limits and counts describe the storage contract.
func AllowAllReportsFirstRefusal(t *testing.T, s storages.MultiStorage) {
	t.Helper()
	ctx := t.Context()
	for _, k := range []string{host(1), host(2)} {
		_, err := s.Allow(ctx, k, 1, period)
		require.NoError(t, err)
	}

	infos, err := s.AllowAll(ctx, []storages.Limit{
		{Key: keyGlobal, Limit: 4, Period: period},
		{Key: host(2), Limit: 1, Period: period},
		{Key: host(1), Limit: 1, Period: period},
	})
	keyErr, ok := errors.AsType[*storages.KeyError](err)
	require.True(t, ok, "%v", err)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, 1, keyErr.Index)
	require.Equal(t, host(2), keyErr.Key)
	require.Equal(t, []int64{4, 0, 0}, []int64{infos[0].Remaining, infos[1].Remaining, infos[2].Remaining})
	require.Positive(t, infos[1].Reset)
	require.Equal(t, int64(0), taken(t, s, keyGlobal))
}

// AllowAllRejectsDuplicateKeys refuses a call that names one key twice with
// [storages.ErrDuplicateKey] and takes nothing.
//
//nolint:mnd // Fixed limits and counts describe the storage contract.
func AllowAllRejectsDuplicateKeys(t *testing.T, s storages.MultiStorage) {
	t.Helper()
	_, err := s.AllowAll(t.Context(), []storages.Limit{
		{Key: keyGlobal, Limit: 1, Period: period},
		{Key: host(1), Limit: 1, Period: period},
		{Key: keyGlobal, Limit: 5, Period: period},
	})
	require.ErrorIs(t, err, storages.ErrDuplicateKey)
	keyErr, ok := errors.AsType[*storages.KeyError](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, 2, keyErr.Index)
	require.NotErrorIs(t, err, storages.ErrLimitExceeded)

	require.Equal(t, int64(0), taken(t, s, keyGlobal))
	require.Equal(t, int64(0), taken(t, s, host(1)))
}

// AllowAllConcurrentAllOrNothing races calls over overlapping key sets, in
// both key orders, and checks that every key admitted exactly what its limit
// allows and that the store holds exactly one slot per admitted call on each
// of its keys: a partial take would leave the other key short.
//
//nolint:mnd // Fixed limits and counts describe the storage contract.
func AllowAllConcurrentAllOrNothing(t *testing.T, s storages.MultiStorage) {
	t.Helper()
	const (
		hosts      = 4
		goroutines = 32
		calls      = 12
	)
	tests := []struct {
		name                   string
		globalLimit, hostLimit int64
	}{
		{name: "host bound", globalLimit: 10_000, hostLimit: 5},
		{name: "global bound", globalLimit: 9, hostLimit: 10_000},
	}
	for ti, tc := range tests {
		global := keyGlobal + "-" + strconv.Itoa(ti)
		hostKey := func(i int) string { return host(ti*hosts + i) }

		var perHost [hosts]atomic.Int64
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Go(func() {
				h := g % hosts
				limits := []storages.Limit{
					{Key: global, Limit: tc.globalLimit, Period: period},
					{Key: hostKey(h), Limit: tc.hostLimit, Period: period},
				}
				if g/hosts%2 == 1 {
					limits[0], limits[1] = limits[1], limits[0]
				}
				for range calls {
					_, err := s.AllowAll(t.Context(), limits)
					if err == nil {
						perHost[h].Add(1)
						continue
					}
					if !errors.Is(err, storages.ErrLimitExceeded) {
						t.Errorf("%s: AllowAll: %v", tc.name, err)
						return
					}
				}
			})
		}
		wg.Wait()

		var total int64
		for h := range hosts {
			n := perHost[h].Load()
			total += n
			require.LessOrEqual(t, n, tc.hostLimit, "%s: host %d over-admitted", tc.name, h)
			require.Equal(t, n, taken(t, s, hostKey(h)), "%s: host %d holds a slot per admitted call", tc.name, h)
		}
		require.Equal(t, min(tc.globalLimit, hosts*min(tc.hostLimit, goroutines/hosts*calls)), total, tc.name)
		require.Equal(t, total, taken(t, s, global), "%s: global holds a slot per admitted call", tc.name)
	}
}
