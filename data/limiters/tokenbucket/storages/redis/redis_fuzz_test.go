// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/redis"

	goredis "github.com/redis/go-redis/v9"
)

func FuzzProvider_Allow(f *testing.F) {
	f.Add("key1", int64(10), int64(60))
	f.Add("special:key", int64(1), int64(1))

	mr, err := miniredis.Run()
	if err != nil {
		f.Fatalf("failed to start miniredis: %v", err)
	}
	f.Cleanup(func() { mr.Close() })

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	p := redis.New(client)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key string, limit, periodSec int64) {
		if key == "" || limit <= 0 || periodSec <= 0 {
			return
		}
		period := time.Duration(periodSec) * time.Second
		if period <= 0 {
			return
		}
		_, _ = p.Allow(ctx, key, limit, period)
	})
}
