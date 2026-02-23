// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"fmt"
	"testing"
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
