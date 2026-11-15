// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/cache/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
)

func BenchmarkProvider_Save(b *testing.B) {
	client, _ := redistest.RedisClient(b)

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
	client, _ := redistest.RedisClient(b)

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
