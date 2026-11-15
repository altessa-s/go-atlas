// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkCache_Get(b *testing.B) {
	c, _ := NewCache[string, int](1000)
	for i := range 1000 {
		c.Put(fmt.Sprintf("key-%d", i), i)
	}

	b.ResetTimer()
	i := 0
	for b.Loop() {
		c.Get(fmt.Sprintf("key-%d", i%1000))
		i++
	}
}

func BenchmarkShardedCache_Get(b *testing.B) {
	c, _ := NewShardedCache[string, int](1000)
	for i := range 1000 {
		c.Put(fmt.Sprintf("key-%d", i), i)
	}

	b.ResetTimer()
	i := 0
	for b.Loop() {
		c.Get(fmt.Sprintf("key-%d", i%1000))
		i++
	}
}

func BenchmarkShardedCache_ConcurrentPut(b *testing.B) {
	c, _ := NewShardedCache[string, int](10000)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Put(fmt.Sprintf("key-%d", i), i)
			i++
		}
	})
}

func BenchmarkCache_GetOrCompute_StringKey(b *testing.B) {
	c, _ := NewCache[string, int](1000)
	ctx := b.Context()

	for b.Loop() {
		_, _ = c.GetOrCompute(ctx, "miss-key", func(ctx context.Context) (int, error) {
			return 42, nil
		})
		c.Remove("miss-key")
	}
}

func BenchmarkCache_GetOrCompute_IntKey(b *testing.B) {
	c, _ := NewCache[int, string](1000)
	ctx := b.Context()

	for b.Loop() {
		_, _ = c.GetOrCompute(ctx, 12345, func(ctx context.Context) (string, error) {
			return "value", nil
		})
		c.Remove(12345)
	}
}

func BenchmarkExpirableCache_PutIfAbsent(b *testing.B) {
	c := NewExpirableCache[string, int](1000, time.Minute)
	c.Put("hit", 1)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = c.PutIfAbsent("hit", 2)
	}
}

func BenchmarkExpirableCache_CompareAndSwap(b *testing.B) {
	c := NewExpirableCache[string, int](1000, time.Minute)
	c.Put("k", 0)
	b.ReportAllocs()
	v := 0
	for b.Loop() {
		if c.CompareAndSwap("k", v, v+1) {
			v++
		}
	}
}

// BenchmarkExpirableCache_PutFull inserts new keys into a full cache of live entries: the earliest-expiry bound skips the sweep, so
// an insert does not scan the entries.
func BenchmarkExpirableCache_PutFull(b *testing.B) {
	c := NewExpirableCache[int, int](10000, time.Hour)
	for i := range 10000 {
		c.Put(i, i)
	}
	b.ReportAllocs()
	i := 10000
	for b.Loop() {
		c.Put(i, i)
		i++
	}
}
