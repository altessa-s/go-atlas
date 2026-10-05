// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"

	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	cuckooredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// redisAddr returns the Redis address, honoring REDIS_ADDR (the integration
// stack in tests/integration publishes Redis Stack on 127.0.0.1:16379).
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

var itSeq atomic.Int64

// newRedisBloomIT connects to a live Redis with the RedisBloom module and
// returns a client plus a per-test key prefix whose keys are dropped on
// cleanup. It skips when Redis or the module is unavailable.
func newRedisBloomIT(t *testing.T, protocol int) (*goredis.Client, string) {
	t.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: redisAddr(), Protocol: protocol})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}

	prefix := "probfilter_it:" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(itSeq.Add(1), 10) + ":"
	if err := client.Do(t.Context(), "BF.RESERVE", prefix+"probe", 0.01, 10).Err(); err != nil {
		t.Skipf("RedisBloom module not available at %s: %v", redisAddr(), err)
	}
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		keys, _ := client.Keys(ctx, "*"+prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
	})
	return client, prefix
}

func valuesLoader(values []string, count int64) probfilter.DataLoader {
	return probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values(values) }, probfilter.WithCount(count))
}

var errITLoad = errors.New("load failed")

// TestRedisFilters_AtomicRebuild_Integration runs against both protocols:
// RedisBloom answers EXISTS/DEL with integers under RESP2 and booleans under
// RESP3 (the go-redis default), and *.INFO with an array or a map.
func TestRedisFilters_AtomicRebuild_Integration(t *testing.T) {
	for _, protocol := range []int{2, 3} {
		t.Run(fmt.Sprintf("RESP%d", protocol), func(t *testing.T) {
			testAtomicRebuild(t, protocol)
		})
	}
}

func testAtomicRebuild(t *testing.T, protocol int) {
	client, prefix := newRedisBloomIT(t, protocol)

	filters := map[string]func() probfilter.RebuildableFilter{
		"bloom": func() probfilter.RebuildableFilter {
			return bloom.New(bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix), bloomredis.WithExpectedItems(1000)))
		},
		"cuckoo": func() probfilter.RebuildableFilter {
			return cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix),
				cuckooredis.WithCapacity(1000), cuckooredis.WithExpansion(2)))
		},
	}

	for name, newFilter := range filters {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			f := newFilter()

			require.NoError(t, f.Add(ctx, "old"))
			require.NoError(t, f.Rebuild(ctx, valuesLoader([]string{"a", "b"}, -1)))

			for _, v := range []string{"a", "b"} {
				ok, err := f.MightExist(ctx, v)
				require.NoError(t, err)
				require.True(t, ok, "%q after rebuild", v)
			}
			ok, err := f.MightExist(ctx, "old")
			require.NoError(t, err)
			require.False(t, ok, "the rebuild replaces the previous contents")
			require.False(t, f.LastRebuild().IsZero())

			liveKey := prefix + map[string]string{"bloom": "bf", "cuckoo": "cf"}[name]
			ttl, err := client.TTL(ctx, liveKey).Result()
			require.NoError(t, err)
			require.Equal(t, time.Duration(-1), ttl, "the staging TTL must not reach the live key")

			stats, err := f.(probfilter.StatsProvider).Stats(ctx)
			require.NoError(t, err)
			require.Positive(t, stats.Capacity, "INFO must be parsed under RESP%d", protocol)
			require.Equal(t, int64(2), stats.ItemCount)

			if d, ok := f.(probfilter.DeletableFilter); ok {
				deleted, err := d.Delete(ctx, "a")
				require.NoError(t, err)
				require.True(t, deleted)
				deleted, err = d.Delete(ctx, "never-added")
				require.NoError(t, err)
				require.False(t, deleted)
				require.NoError(t, f.Add(ctx, "a"))
			}

			// A failing loader leaves the live filter and no staging key behind.
			last := f.LastRebuild()
			failing := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					if yield("c", nil) {
						yield("", errITLoad)
					}
				}
			})
			require.ErrorIs(t, f.Rebuild(ctx, failing), errITLoad)
			ok, err = f.MightExist(ctx, "a")
			require.NoError(t, err)
			require.True(t, ok, "a failed rebuild keeps the previous contents")
			require.Equal(t, last, f.LastRebuild())

			require.Empty(t, stagingKeys(t, client, prefix), "aborted and committed rebuilds leave no staging keys")
		})
	}
}

func TestRedisStaging_TTL_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()
	s := bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix))

	st, err := s.Stage(ctx, 10)
	require.NoError(t, err)
	staging, err := client.Keys(ctx, "*"+prefix+"*:staging:*").Result()
	require.NoError(t, err)
	require.Len(t, staging, 1)
	ttl, err := client.TTL(ctx, staging[0]).Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "a staging key must carry a TTL so a crashed rebuild cannot leak it")
	require.NoError(t, st.Abort(ctx))
}

// TestRedisStaging_VanishedKeyFailsRebuild_Integration deletes the staging
// key after its first batch: the next NOCREATE batch must fail instead of
// recreating it, and the live filter must stay as it was.
func TestRedisStaging_VanishedKeyFailsRebuild_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()

	filters := map[string]probfilter.RebuildableFilter{
		"bloom":  bloom.New(bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix))),
		"cuckoo": cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix))),
	}
	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, f.Add(ctx, "old"))

			values := make([]string, 2500)
			for i := range values {
				values[i] = fmt.Sprintf("v-%d", i)
			}
			loader := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					for i, v := range values {
						if i == 1500 { // after the first staging batch was sent
							staging, err := client.Keys(ctx, "*"+prefix+"*:staging:*").Result()
							require.NoError(t, err)
							require.NotEmpty(t, staging)
							require.NoError(t, client.Del(ctx, staging...).Err())
						}
						if !yield(v, nil) {
							return
						}
					}
				}
			})
			// Count known: values stream straight into staging batches.
			err := f.Rebuild(ctx, countedLoader{DataLoader: loader, n: int64(len(values))})
			require.Error(t, err)

			ok, err := f.MightExist(ctx, "old")
			require.NoError(t, err)
			require.True(t, ok, "the live filter must be unchanged")
			ok, err = f.MightExist(ctx, "v-0")
			require.NoError(t, err)
			require.False(t, ok, "no partial rebuild may be committed")

			require.Empty(t, stagingKeys(t, client, prefix), "the staging key must not be recreated")
		})
	}
}

// countedLoader reports a fixed count for a wrapped loader.
type countedLoader struct {
	probfilter.DataLoader
	n int64
}

func (l countedLoader) Count(context.Context) (int64, error) { return l.n, nil }

// TestRedisLiveFilter_ReservedWithConfig_Integration checks that an ordinary
// first write creates the live filter with the configured parameters, not
// with RedisBloom's implicit defaults.
func TestRedisLiveFilter_ReservedWithConfig_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()

	cf := cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix),
		cuckooredis.WithCapacity(5000), cuckooredis.WithExpansion(4)))
	require.NoError(t, cf.Add(ctx, "a"))
	info, err := client.Do(ctx, "CF.INFO", prefix+"cf").Result()
	require.NoError(t, err)
	require.Equal(t, int64(4), infoField(t, info, "Expansion rate"))

	bf := bloom.New(bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix), bloomredis.WithExpectedItems(7000)))
	require.NoError(t, bf.AddBatch(ctx, slices.Values([]string{"a", "b"})))
	info, err = client.Do(ctx, "BF.INFO", prefix+"bf").Result()
	require.NoError(t, err)
	require.Equal(t, int64(7000), infoField(t, info, "Capacity"))
}

func infoField(t *testing.T, info any, name string) int64 {
	t.Helper()
	switch v := info.(type) {
	case map[any]any:
		n, ok := v[name].(int64)
		require.True(t, ok, "field %q in %v", name, v)
		return n
	case []any:
		for i := 0; i+1 < len(v); i += 2 {
			if v[i] == name {
				return v[i+1].(int64)
			}
		}
	}
	t.Fatalf("field %q not found in %v", name, info)
	return 0
}

// stagingKeys lists the rebuild staging keys of filters under prefix and
// checks that every commit marker expires.
func stagingKeys(t *testing.T, client *goredis.Client, prefix string) []string {
	t.Helper()
	markers, err := client.Keys(t.Context(), "*"+prefix+"*}:committed:*").Result()
	require.NoError(t, err)
	for _, k := range markers {
		ttl, err := client.TTL(t.Context(), k).Result()
		require.NoError(t, err)
		require.Positive(t, ttl, "commit marker %q must expire", k)
	}
	staging, err := client.Keys(t.Context(), "*"+prefix+"*}:staging:*").Result()
	require.NoError(t, err)
	return staging
}

// TestRedisCuckoo_DeleteGeneration_Integration checks generation-bound deletes
// against RedisBloom: a missing filter is not an error, a delete works before
// and after a rebuild, and the rebuild records a generation token.
func TestRedisCuckoo_DeleteGeneration_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()
	f := cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix)))

	deleted, err := f.Delete(ctx, "nothing-yet")
	require.NoError(t, err, "deleting from a filter that does not exist yet is not an error")
	require.False(t, deleted)

	require.NoError(t, f.Add(ctx, "a"))
	deleted, err = f.Delete(ctx, "a")
	require.NoError(t, err)
	require.True(t, deleted)

	require.NoError(t, f.Rebuild(ctx, valuesLoader([]string{"b", "c"}, 2)))
	genKeys, err := client.Keys(ctx, "*"+prefix+"cf}:generation:*").Result()
	require.NoError(t, err)
	require.Len(t, genKeys, 1)
	token, err := client.Get(ctx, genKeys[0]).Result()
	require.NoError(t, err)
	require.NotEmpty(t, token, "a commit records a new generation")

	deleted, err = f.Delete(ctx, "b")
	require.NoError(t, err)
	require.True(t, deleted)
	ok, err := f.MightExist(ctx, "c")
	require.NoError(t, err)
	require.True(t, ok)
}

// pausingLoader yields values; before yielding it signals started and waits
// for resume.
type pausingLoader struct {
	values  []string
	started chan struct{}
	resume  chan struct{}
}

func (l *pausingLoader) Count(context.Context) (int64, error) { return -1, nil }

func (l *pausingLoader) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		close(l.started)
		<-l.resume
		for _, v := range l.values {
			if !yield(v, nil) {
				return
			}
		}
	}
}

// TestRedisConcurrentRebuilds_Integration runs two "processes" (two filter
// instances over one Redis key) rebuilding concurrently (OIDC-016):
//
//   - while A rebuilds, B's rebuild is refused and B's source is never read;
//   - when A stalls until its lease is lost, B rebuilds and publishes a newer
//     snapshot, and A's later commit is rejected: B's contents stay live.
func TestRedisConcurrentRebuilds_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()

	type pair struct{ a, b probfilter.RebuildableFilter }
	filters := map[string]func() pair{
		"bloom": func() pair {
			mk := func() probfilter.RebuildableFilter {
				return bloom.New(bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix), bloomredis.WithExpectedItems(1000)))
			}
			return pair{mk(), mk()}
		},
		"cuckoo": func() pair {
			mk := func() probfilter.RebuildableFilter {
				return cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix), cuckooredis.WithCapacity(1000)))
			}
			return pair{mk(), mk()}
		},
	}

	for name, mk := range filters {
		t.Run(name, func(t *testing.T) {
			p := mk()
			leaseKey := func() string {
				keys, err := client.Keys(ctx, "*"+prefix+"*:rebuild-lease:*").Result()
				require.NoError(t, err)
				require.Len(t, keys, 1)
				return keys[0]
			}

			// 1. B is refused while A holds the lease.
			loaderA := &pausingLoader{values: []string{"a-1"}, started: make(chan struct{}), resume: make(chan struct{})}
			doneA := make(chan error, 1)
			go func() { doneA <- p.a.Rebuild(ctx, loaderA) }()
			<-loaderA.started

			var bRead atomic.Bool
			loaderB := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
				bRead.Store(true)
				return func(yield func(string, error) bool) { yield("b-1", nil) }
			})
			require.ErrorIs(t, p.b.Rebuild(ctx, loaderB), probfilter.ErrRebuildInProgress)
			require.False(t, bRead.Load(), "a refused rebuild must not read its source")

			close(loaderA.resume)
			require.NoError(t, <-doneA)
			requireMember(t, p.a, "a-1")

			// 2. A loads an older snapshot and stalls until its lease is lost;
			// B publishes a newer one; A's commit must be rejected.
			older := make([]string, 20)
			for i := range older {
				older[i] = fmt.Sprintf("older-%d", i)
			}
			loaderA = &pausingLoader{values: older, started: make(chan struct{}), resume: make(chan struct{})}
			go func() { doneA <- p.a.Rebuild(ctx, loaderA) }()
			<-loaderA.started
			require.NoError(t, client.Del(ctx, leaseKey()).Err()) // A's lease expired

			require.NoError(t, p.b.Rebuild(ctx, valuesLoader([]string{"newer"}, -1)))
			close(loaderA.resume)
			require.ErrorIs(t, <-doneA, probfilter.ErrRebuildSuperseded)

			requireMember(t, p.b, "newer")
			present := 0
			for _, v := range older {
				ok, err := p.b.MightExist(ctx, v)
				require.NoError(t, err)
				if ok {
					present++
				}
			}
			// A few may match as false positives; a published stale snapshot
			// would make all of them match.
			require.Less(t, present, 5, "the stale snapshot must not overwrite the newer one")
			require.Empty(t, stagingKeys(t, client, prefix), "the superseded staging key is removed")
		})
	}
}

func requireMember(t *testing.T, f probfilter.Filter, v string) {
	t.Helper()
	ok, err := f.MightExist(t.Context(), v)
	require.NoError(t, err)
	require.True(t, ok, "MightExist(%q)", v)
}

// TestRedisRebuildCommitted_Integration checks the shared rebuild state two
// processes see through RedisBloom: false until one of them commits a
// rebuild, true for both afterwards, and false again once the filter key was
// deleted and recreated by a write.
func TestRedisRebuildCommitted_Integration(t *testing.T) {
	client, prefix := newRedisBloomIT(t, 3)
	ctx := t.Context()

	type filter interface {
		probfilter.RebuildableFilter
		probfilter.RebuildCommitReporter
	}
	filters := map[string]func() filter{
		"bloom": func() filter {
			return bloom.New(bloomredis.New(client, "bf", bloomredis.WithKeyPrefix(prefix), bloomredis.WithExpectedItems(1000)))
		},
		"cuckoo": func() filter {
			return cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix), cuckooredis.WithCapacity(1000)))
		},
	}
	committed := func(t *testing.T, f filter) bool {
		t.Helper()
		ok, err := f.RebuildCommitted(ctx)
		require.NoError(t, err)
		return ok
	}

	for name, mk := range filters {
		t.Run(name, func(t *testing.T) {
			a, b := mk(), mk()
			require.NoError(t, b.Add(ctx, "added")) // reserves the filter; b's reservation latches
			require.False(t, committed(t, a))
			require.False(t, committed(t, b))

			require.NoError(t, a.Rebuild(ctx, valuesLoader([]string{"x"}, -1)))
			require.True(t, committed(t, a))
			require.True(t, committed(t, b), "a rebuild committed by another process counts")
			require.True(t, b.LastRebuild().IsZero(), "b never rebuilt itself")

			liveKey := prefix + map[string]string{"bloom": "bf", "cuckoo": "cf"}[name]
			require.NoError(t, client.Del(ctx, liveKey).Err())
			require.False(t, committed(t, b))

			require.NoError(t, b.Add(ctx, "after-delete"), "a missing filter is recreated by the write")
			requireMember(t, b, "after-delete")
			require.False(t, committed(t, a), "a recreated filter does not hold the committed rebuild")
			require.NoError(t, b.AddBatch(ctx, slices.Values([]string{"y", "z"})))
			requireMember(t, b, "z")

			require.NoError(t, b.Rebuild(ctx, valuesLoader([]string{"x"}, -1)))
			require.True(t, committed(t, a))
		})
	}
}

// TestRedisCuckoo_FullFilterRejectsWrites_Integration fills a non-growing
// Redis Cuckoo filter: a write that does not fit must fail under both
// protocols — CF.INSERT reports it as -1 under RESP2 and as false under
// RESP3 — instead of silently dropping the value.
func TestRedisCuckoo_FullFilterRejectsWrites_Integration(t *testing.T) {
	for _, protocol := range []int{2, 3} {
		t.Run(fmt.Sprintf("RESP%d", protocol), func(t *testing.T) {
			client, prefix := newRedisBloomIT(t, protocol)
			ctx := t.Context()
			// A tiny, non-growing filter; the storage's own reserve finds it existing.
			require.NoError(t, client.Do(ctx, "CF.RESERVE", prefix+"cf", 2, "BUCKETSIZE", 1, "EXPANSION", 0).Err())
			f := cuckoo.New(cuckooredis.New(client, "cf", cuckooredis.WithKeyPrefix(prefix), cuckooredis.WithCapacity(100)))

			var addErr error
			for i := 0; i < 50 && addErr == nil; i++ {
				addErr = f.Add(ctx, fmt.Sprintf("v-%d", i))
			}
			require.ErrorIs(t, addErr, redisfilter.ErrItemRejected, "a full filter must reject Add")

			values := make([]string, 50)
			for i := range values {
				values[i] = fmt.Sprintf("w-%d", i)
			}
			require.ErrorIs(t, f.AddBatch(ctx, slices.Values(values)), redisfilter.ErrItemRejected, "a full filter must reject AddBatch")
		})
	}
}
