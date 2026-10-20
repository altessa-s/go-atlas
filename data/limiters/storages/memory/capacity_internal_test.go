// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// evictionPolicies covers both policies at the cap.
var evictionPolicies = []struct {
	name     string
	eviction Eviction
}{{"lru", EvictionLRU}, {"reject", EvictionReject}}

// TestProvider_AtCapacity_ReclaimsExpiredBeforeLive is the eviction-bypass
// regression for the LRU policy: a full provider must reclaim windows that
// already expired before it evicts the least recently used live one. The
// hot key sits at the LRU tail with its single slot taken; evicting it would
// hand it a fresh window.
func TestProvider_AtCapacity_ReclaimsExpiredBeforeLive(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		for _, pol := range evictionPolicies {
			t.Run(pol.name+"/shards="+strconv.Itoa(shards), func(t *testing.T) {
				t.Parallel()
				p := New(WithMaxBuckets(3), WithShards(shards), WithEviction(pol.eviction))
				ctx := t.Context()
				_, err := p.Allow(ctx, "hot", 1, time.Hour)
				require.NoError(t, err)
				for _, k := range []string{"short-1", "short-2"} {
					_, err := p.Allow(ctx, k, 1, time.Millisecond)
					require.NoError(t, err)
				}
				time.Sleep(5 * time.Millisecond)

				_, err = p.Allow(ctx, "new", 1, time.Hour)
				require.NoError(t, err)
				require.True(t, p.tracked("hot"), "the live window must outlive the expired ones")
				_, err = p.Allow(ctx, "hot", 1, time.Hour)
				require.ErrorIs(t, err, storages.ErrLimitExceeded, "the hot key must keep its limit")
				require.False(t, p.tracked("short-1"))
				require.False(t, p.tracked("short-2"))
				require.Equal(t, 2, p.total(t))
			})
		}
	}
}

// TestProvider_EvictionReject_KeyChurnKeepsLimit is the eviction-bypass
// regression for the reject policy: a flood of new keys cannot push a key
// out of a full provider and reset its limit. New keys fail with
// ErrCapacityExceeded, which is not ErrLimitExceeded, and tracked keys keep
// working.
func TestProvider_EvictionReject_KeyChurnKeepsLimit(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			p := New(WithMaxBuckets(2), WithShards(shards), WithEviction(EvictionReject))
			ctx := t.Context()
			_, err := p.Allow(ctx, "victim", 1, time.Hour)
			require.NoError(t, err)
			_, err = p.Allow(ctx, "other", 100, time.Hour)
			require.NoError(t, err)

			for i := range 200 {
				info, err := p.Allow(ctx, "flood-"+strconv.Itoa(i), 1, time.Hour)
				require.ErrorIs(t, err, ErrCapacityExceeded)
				require.NotErrorIs(t, err, storages.ErrLimitExceeded)
				require.Nil(t, info)
			}

			_, err = p.Allow(ctx, "victim", 1, time.Hour)
			require.ErrorIs(t, err, storages.ErrLimitExceeded, "churn must not reset the victim's window")
			_, err = p.Allow(ctx, "other", 100, time.Hour)
			require.NoError(t, err, "tracked keys keep working at the cap")
			require.Equal(t, 2, p.total(t))
		})
	}
}

// TestProvider_EvictionReject_AdmitsOnceRoomFrees pins the ways a full
// reject-mode provider gets room back: an expired window, a reset window and
// an idle bucket removed by cleanup.
func TestProvider_EvictionReject_AdmitsOnceRoomFrees(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		period time.Duration
		free   func(t *testing.T, p *Provider)
	}{
		{name: "expired window", period: time.Millisecond, free: func(*testing.T, *Provider) { time.Sleep(5 * time.Millisecond) }},
		{name: "reset window", period: time.Hour, free: func(t *testing.T, p *Provider) { require.NoError(t, p.Reset(t.Context(), "old")) }},
		{name: "idle cleanup", period: time.Hour, free: func(_ *testing.T, p *Provider) {
			time.Sleep(5 * time.Millisecond)
			p.RunCleanup()
		}},
	}
	for _, shards := range testShardCounts {
		for _, tc := range tests {
			t.Run(tc.name+"/shards="+strconv.Itoa(shards), func(t *testing.T) {
				t.Parallel()
				p := New(WithMaxBuckets(1), WithShards(shards), WithEviction(EvictionReject), WithMaxIdleTime(time.Millisecond))
				ctx := t.Context()
				_, err := p.Allow(ctx, "old", 1, tc.period)
				require.NoError(t, err)
				_, err = p.Allow(ctx, "new", 1, time.Hour)
				require.ErrorIs(t, err, ErrCapacityExceeded)

				tc.free(t, p)
				_, err = p.Allow(ctx, "new", 1, time.Hour)
				require.NoError(t, err)
				require.Equal(t, 1, p.total(t))
			})
		}
	}
}

// TestProvider_AllowAll_AtCapacity pins new keys of a multi-key call at the
// cap: the reject policy fails the whole call and takes nothing, the LRU
// policy evicts a bucket that is not part of the call even when a key of the
// call is the least recently used one.
func TestProvider_AllowAll_AtCapacity(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		for _, pol := range evictionPolicies {
			t.Run(pol.name+"/shards="+strconv.Itoa(shards), func(t *testing.T) {
				t.Parallel()
				p := New(WithMaxBuckets(3), WithShards(shards), WithEviction(pol.eviction))
				ctx := t.Context()
				for _, k := range []string{"member", "bystander-1", "bystander-2"} {
					_, err := p.Allow(ctx, k, 2, time.Hour)
					require.NoError(t, err)
				}

				limits := []storages.Limit{
					{Key: "member", Limit: 2, Period: time.Hour},
					{Key: "new", Limit: 2, Period: time.Hour},
				}
				_, err := p.AllowAll(ctx, limits)
				if pol.eviction == EvictionReject {
					require.ErrorIs(t, err, ErrCapacityExceeded)
					keyErr, ok := errors.AsType[*storages.KeyError](err)
					require.True(t, ok, "%v", err)
					require.Equal(t, 1, keyErr.Index)
					require.False(t, p.tracked("new"))
					info, err := p.Allow(ctx, "member", 2, time.Hour)
					require.NoError(t, err)
					require.Equal(t, int64(0), info.Remaining, "the failed call must not take a member slot")
					return
				}
				require.NoError(t, err)
				require.True(t, p.tracked("member"))
				require.True(t, p.tracked("new"))
				require.False(t, p.tracked("bystander-1"), "the LRU bucket outside the call is evicted")
				_, err = p.Allow(ctx, "member", 2, time.Hour)
				require.ErrorIs(t, err, storages.ErrLimitExceeded, "the member keeps the slot the call took")
				require.Equal(t, 3, p.total(t))
			})
		}
	}
}

func TestProvider_AllowAll_MoreKeysThanCapacity(t *testing.T) {
	t.Parallel()
	p := New(WithMaxBuckets(1))
	_, err := p.AllowAll(t.Context(), []storages.Limit{
		{Key: "a", Limit: 1, Period: time.Hour},
		{Key: "b", Limit: 1, Period: time.Hour},
	})
	require.ErrorIs(t, err, ErrCapacityExceeded)
	keyErr, ok := errors.AsType[*storages.KeyError](err)
	require.True(t, ok, "%v", err)
	require.Equal(t, 1, keyErr.Index)
	require.Equal(t, 0, p.total(t))
}

// TestProvider_AllowAll_ConcurrentOverlappingAtCapacity races multi-key
// calls over overlapping key sets in random orders with single-key calls,
// resets and cleanup on a small sharded provider, so calls take the
// per-shard, all-shard, eviction and reject paths together. It must finish
// (no lock-order deadlock) and keep the bucket accounting exact; the race
// detector checks the locking.
func TestProvider_AllowAll_ConcurrentOverlappingAtCapacity(t *testing.T) {
	t.Parallel()
	for _, pol := range evictionPolicies {
		t.Run(pol.name, func(t *testing.T) {
			t.Parallel()
			const (
				maxBuckets = 6
				pool       = 9
				limit      = 3
				goroutines = 16
				calls      = 400
			)
			p := New(WithMaxBuckets(maxBuckets), WithShards(4), WithEviction(pol.eviction), WithMaxIdleTime(time.Millisecond))
			ctx := t.Context()
			var admitted [pool]atomic.Int64

			var wg sync.WaitGroup
			for g := range goroutines {
				wg.Go(func() {
					rng := rand.New(rand.NewPCG(uint64(g), 1)) // #nosec G404 -- test workload shape
					for i := range calls {
						perm := rng.Perm(pool)[:3]
						limits := make([]storages.Limit, len(perm))
						for j, k := range perm {
							limits[j] = storages.Limit{Key: "k" + strconv.Itoa(k), Limit: limit, Period: time.Hour}
						}
						switch i % 10 {
						case 7:
							_, _ = p.Allow(ctx, limits[0].Key, limit, time.Hour)
							continue
						case 8:
							_ = p.Reset(ctx, limits[0].Key)
							continue
						case 9:
							p.RunCleanup()
							continue
						}
						_, err := p.AllowAll(ctx, limits)
						switch {
						case err == nil:
							for _, k := range perm {
								admitted[k].Add(1)
							}
						case errors.Is(err, storages.ErrLimitExceeded), errors.Is(err, ErrCapacityExceeded):
						default:
							t.Errorf("AllowAll: %v", err)
							return
						}
					}
				})
			}
			wg.Wait()
			require.LessOrEqual(t, p.total(t), maxBuckets)
			var total int64
			for k := range pool {
				total += admitted[k].Load()
			}
			require.Positive(t, total, "the workload must admit some calls")
		})
	}
}

// TestProvider_AllowAll_NeverOverAdmitsAcrossShards hammers two keys that
// live in different shards, so every call locks two shards, and checks that
// each key admitted exactly its limit.
func TestProvider_AllowAll_NeverOverAdmitsAcrossShards(t *testing.T) {
	t.Parallel()
	p := New(WithShards(8))
	keys := p.keysInShards(2, true)
	const limit = 25
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for g := range 32 {
		wg.Go(func() {
			limits := []storages.Limit{
				{Key: keys[g%2], Limit: limit, Period: time.Hour},
				{Key: keys[1-g%2], Limit: limit, Period: time.Hour},
			}
			for range 10 {
				if _, err := p.AllowAll(t.Context(), limits); err == nil {
					admitted.Add(1)
				}
			}
		})
	}
	wg.Wait()
	require.Equal(t, int64(limit), admitted.Load())
	for _, k := range keys {
		require.Equal(t, limit, p.recorded(k), k)
	}
}

// recorded returns the number of requests in key's window, expired or not.
func (p *Provider) recorded(key string) int {
	sh := p.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	b := sh.buckets[key]
	return len(b.requests) - b.head
}

// TestProvider_AllowAll_ShortenedPeriodKeepsReclaimBound pins the reclaim
// bound across a failed call: checking a window under a shorter period than
// it was recorded with can empty it, and a capacity failure that follows
// must not leave the empty bucket unreclaimable until the old, longer
// window would have expired.
func TestProvider_AllowAll_ShortenedPeriodKeepsReclaimBound(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			p := New(WithMaxBuckets(2), WithShards(shards), WithEviction(EvictionReject))
			ctx := t.Context()
			for _, k := range []string{"a", "b"} {
				_, err := p.Allow(ctx, k, 1, time.Hour)
				require.NoError(t, err)
			}
			time.Sleep(5 * time.Millisecond)

			_, err := p.AllowAll(ctx, []storages.Limit{
				{Key: "a", Limit: 1, Period: time.Millisecond},
				{Key: "new", Limit: 1, Period: time.Hour},
			})
			require.ErrorIs(t, err, ErrCapacityExceeded)

			_, err = p.Allow(ctx, "later", 1, time.Hour)
			require.NoError(t, err, "a's window is empty under its new period and must be reclaimed")
			require.False(t, p.tracked("a"))
			require.True(t, p.tracked("b"))
			require.Equal(t, 2, p.total(t))
		})
	}
}

// TestProvider_ReserveSlots_AllOrNothing pins the reservation primitive
// behind multi-key admission: without every shard lock a call reserves all
// the slots its new keys need in one step or none, so a racing call never
// sees, and fails on, a part of it. Another call's in-flight reservation is
// simulated by the count.
func TestProvider_ReserveSlots_AllOrNothing(t *testing.T) {
	t.Parallel()
	p := New(WithMaxBuckets(2))
	p.count.Store(1)
	require.Equal(t, 0, p.reserveSlots(2, false))
	require.Equal(t, int64(1), p.count.Load(), "a failed reservation must hold no slot")
	require.Equal(t, 1, p.reserveSlots(1, false))
	require.Equal(t, int64(2), p.count.Load())

	p.count.Store(1)
	require.Equal(t, 1, p.reserveSlots(2, true), "a partial reservation takes the free slots")
	require.Equal(t, int64(2), p.count.Load())

	unlimited := New(WithMaxBuckets(0))
	require.Equal(t, 5, unlimited.reserveSlots(5, false))
}

// TestProvider_AllowAll_CompetingReservations races two calls that each
// need every slot of an empty reject-mode provider, on disjoint shards so
// neither waits for the other's locks. Serialized atomic admissions always
// admit one of them; partial reservations visible to the other call could
// make both fail.
func TestProvider_AllowAll_CompetingReservations(t *testing.T) {
	t.Parallel()
	for range 500 {
		p := New(WithMaxBuckets(2), WithShards(8), WithEviction(EvictionReject))
		keys := p.keysInShards(4, true)
		ctx := t.Context()
		start := make(chan struct{})
		var errs [2]error
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Go(func() {
				<-start
				_, errs[i] = p.AllowAll(ctx, []storages.Limit{
					{Key: keys[2*i], Limit: 1, Period: time.Hour},
					{Key: keys[2*i+1], Limit: 1, Period: time.Hour},
				})
			})
		}
		close(start)
		wg.Wait()
		require.False(t, errs[0] != nil && errs[1] != nil, "both calls failed: %v, %v", errs[0], errs[1])
		require.Equal(t, 2, p.total(t))
	}
}

// TestProvider_AllowAll_CapacityErrorNamesNewKey pins the index of a
// capacity failure: it names the first new key without room, wherever the
// tracked keys of the call sit, including calls with more keys than the
// cap.
func TestProvider_AllowAll_CapacityErrorNamesNewKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		maxBuckets int
		existing   []string
		call       []string
		want       int
	}{
		{name: "new first", maxBuckets: 2, existing: []string{"a", "b"}, call: []string{"new", "a", "b"}, want: 0},
		{name: "new middle", maxBuckets: 2, existing: []string{"a", "b"}, call: []string{"a", "new", "b"}, want: 1},
		{name: "new last", maxBuckets: 2, existing: []string{"a", "b"}, call: []string{"a", "b", "new"}, want: 2},
		{name: "room for two new", maxBuckets: 3, existing: []string{"a"}, call: []string{"n1", "a", "n2", "n3"}, want: 3},
	}
	for _, shards := range testShardCounts {
		for _, pol := range evictionPolicies {
			for _, tc := range tests {
				t.Run(pol.name+"/shards="+strconv.Itoa(shards)+"/"+tc.name, func(t *testing.T) {
					t.Parallel()
					p := New(WithMaxBuckets(tc.maxBuckets), WithShards(shards), WithEviction(pol.eviction))
					ctx := t.Context()
					for _, k := range tc.existing {
						_, err := p.Allow(ctx, k, 5, time.Hour)
						require.NoError(t, err)
					}
					limits := make([]storages.Limit, len(tc.call))
					for i, k := range tc.call {
						limits[i] = storages.Limit{Key: k, Limit: 5, Period: time.Hour}
					}

					_, err := p.AllowAll(ctx, limits)
					require.ErrorIs(t, err, ErrCapacityExceeded)
					keyErr, ok := errors.AsType[*storages.KeyError](err)
					require.True(t, ok, "%v", err)
					require.Equal(t, tc.want, keyErr.Index)
					require.Equal(t, tc.call[tc.want], keyErr.Key)
					for _, k := range tc.existing {
						require.Equal(t, 1, p.recorded(k), "a failed call takes no slot on %s", k)
					}
					require.Equal(t, len(tc.existing), p.total(t))
				})
			}
		}
	}
}
