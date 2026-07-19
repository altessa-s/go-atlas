// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cfredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
)

func BenchmarkNew(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)

	for b.Loop() {
		_ = cfredis.New(client, "bench-filter")
	}
}

func BenchmarkStorage_Close(b *testing.B) {
	client, _ := testhelpers.RedisClient(b)

	s := cfredis.New(client, "bench-filter")
	ctx := b.Context()

	for b.Loop() {
		_ = s.Close(ctx)
	}
}
