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
