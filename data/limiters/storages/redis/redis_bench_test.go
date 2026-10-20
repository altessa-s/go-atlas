// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/redis"

	goredis "github.com/redis/go-redis/v9"
)

func benchSetup(b *testing.B) *redis.Provider {
	b.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	b.Cleanup(func() { mr.Close() })
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return redis.New(client)
}

func BenchmarkProvider_Allow(b *testing.B) {
	p := benchSetup(b)
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Allow(ctx, "bench-key", 1000000, time.Minute)
	}
}

func BenchmarkProvider_Reset(b *testing.B) {
	p := benchSetup(b)
	ctx := b.Context()

	for b.Loop() {
		_ = p.Reset(ctx, "bench-key")
	}
}

// BenchmarkProvider_AllowAll measures a per-host plus global admission in
// one script round trip.
func BenchmarkProvider_AllowAll(b *testing.B) {
	p := benchSetup(b)
	ctx := b.Context()
	limits := []storages.Limit{
		{Key: "{bench}:host", Limit: 1_000_000, Period: time.Minute},
		{Key: "{bench}:global", Limit: 1_000_000, Period: time.Minute},
	}

	b.ReportAllocs()
	for b.Loop() {
		_, _ = p.AllowAll(ctx, limits)
	}
}
