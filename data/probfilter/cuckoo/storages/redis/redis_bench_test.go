// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	cfredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func BenchmarkNew(b *testing.B) {
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })

	for b.Loop() {
		_ = cfredis.New(client, "bench-filter")
	}
}

func BenchmarkStorage_Close(b *testing.B) {
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })

	s := cfredis.New(client, "bench-filter")
	ctx := b.Context()

	for b.Loop() {
		_ = s.Close(ctx)
	}
}
