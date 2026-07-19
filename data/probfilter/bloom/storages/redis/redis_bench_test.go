// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	bfredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
)

func BenchmarkStorage_SetLastRebuild(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)

	s := bfredis.New(client, "bench-bloom")
	now := time.Now()

	for b.Loop() {
		s.SetLastRebuild(now)
	}
}

func BenchmarkStorage_LastRebuild(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)

	s := bfredis.New(client, "bench-bloom")
	s.SetLastRebuild(time.Now())

	for b.Loop() {
		_ = s.LastRebuild()
	}
}
