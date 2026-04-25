// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/cache/providers/redis"

	goredis "github.com/redis/go-redis/v9"
)

func BenchmarkProvider_Save(b *testing.B) {
	mr := miniredis.RunT(b)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	ctx := b.Context()
	value := []byte("benchmark-value")

	b.ResetTimer()
	for b.Loop() {
		key := "bench-key"
		err := provider.Save(ctx, key, value, 10*time.Second)
		if err != nil {
			b.Fatalf("Save() failed: %v", err)
		}
	}
}

func BenchmarkProvider_Get(b *testing.B) {
	mr := miniredis.RunT(b)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	ctx := b.Context()
	key := "bench-key"
	value := []byte("benchmark-value")

	// Setup: save a value first
	err := provider.Save(ctx, key, value, 0)
	if err != nil {
		b.Fatalf("Setup Save() failed: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := provider.Get(ctx, key)
		if err != nil {
			b.Fatalf("Get() failed: %v", err)
		}
	}
}
