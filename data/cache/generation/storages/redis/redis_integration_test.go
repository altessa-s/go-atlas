// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/time/clock"
	"github.com/altessa-s/go-atlas/data/cache/generation"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/storagetest"

	goredis "github.com/redis/go-redis/v9"
)

// liveSeq makes the key prefix of every live backend unique.
var liveSeq atomic.Int64

// newLiveBackend is the storagetest harness over the Redis at REDIS_ADDR: n
// replicas, each with its own client, under a key prefix unique to the
// backend (dropped on cleanup). The server clock cannot be controlled, so
// SetSeedTime is nil. It skips when REDIS_ADDR is unset or unreachable.
func newLiveBackend(tb testing.TB, n int) storagetest.Backend {
	tb.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		tb.Skip("REDIS_ADDR not set")
	}
	prefix := "generation_it_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(liveSeq.Add(1), 10) + ":"
	fake := clock.NewFake(epoch)
	stores := make([]generation.Store, n)
	for i := range n {
		client := goredis.NewClient(&goredis.Options{Addr: addr})
		tb.Cleanup(func() { _ = client.Close() })
		if i == 0 {
			if err := client.Ping(tb.Context()).Err(); err != nil {
				tb.Skipf("redis not reachable at %s: %v", addr, err)
			}
			tb.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), 5*time.Second)
				defer cancel()
				keys, _ := client.Keys(ctx, prefix+"*").Result()
				if len(keys) > 0 {
					_ = client.Del(ctx, keys...).Err()
				}
			})
		}
		stores[i] = redis.New(client, redis.WithKeyPrefix(prefix), redis.WithClock(fake), redis.WithMaxStaleness(staleness))
	}
	return storagetest.Backend{Stores: stores, Advance: fake.Advance}
}

func TestStore_ContractLiveRedis(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newLiveBackend, staleness)
}
