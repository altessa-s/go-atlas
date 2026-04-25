// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	bfredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func BenchmarkStorage_SetLastRebuild(b *testing.B) {
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "bench-bloom")
	now := time.Now()

	for b.Loop() {
		s.SetLastRebuild(now)
	}
}

func BenchmarkStorage_LastRebuild(b *testing.B) {
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "bench-bloom")
	s.SetLastRebuild(time.Now())

	for b.Loop() {
		_ = s.LastRebuild()
	}
}
