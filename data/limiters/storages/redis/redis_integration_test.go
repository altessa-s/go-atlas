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

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/redis"
	"github.com/altessa-s/go-atlas/data/limiters/storages/storagetest"

	goredis "github.com/redis/go-redis/v9"
)

// liveSeq makes the key prefix of every live store unique.
var liveSeq atomic.Int64

// newLiveStore returns a provider over the Redis at REDIS_ADDR under a key
// prefix unique to the store, whose keys are dropped on cleanup. It skips
// when REDIS_ADDR is unset or unreachable.
func newLiveStore(tb testing.TB) storages.MultiStorage {
	tb.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		tb.Skip("REDIS_ADDR not set")
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	tb.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(tb.Context()).Err(); err != nil {
		tb.Skipf("redis not reachable at %s: %v", addr, err)
	}
	prefix := "limiters_it_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(liveSeq.Add(1), 10) + ":"
	tb.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(tb.Context()), 5*time.Second)
		defer cancel()
		keys, _ := client.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
	})
	return redis.New(client, redis.WithKeyPrefix(prefix))
}

func TestProvider_MultiStorageContractLiveRedis(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, newLiveStore)
}
