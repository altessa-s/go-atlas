// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
	"github.com/altessa-s/go-atlas/data/cache/storages/lru"
)

// BenchmarkGetWithFallbackT_Generation measures a hit on a generation-scoped
// cache: one generation lookup plus the plain hit path.
func BenchmarkGetWithFallbackT_Generation(b *testing.B) {
	storage, err := lru.New(128)
	if err != nil {
		b.Fatal(err)
	}
	c := cache.New(storage, cache.WithGeneration(memory.New(), "catalog"))
	ctx := b.Context()
	fill := func(context.Context) (item, time.Duration, error) {
		return item{Name: "v"}, cache.TTLUseDefault, nil
	}
	_, _ = cache.GetWithFallbackT(ctx, c, "k", fill)
	for b.Loop() {
		_, _ = cache.GetWithFallbackT(ctx, c, "k", fill)
	}
}

// BenchmarkReadThrough_Hit measures a generational read-through hit: one
// generation lookup, the key composition and a decode.
func BenchmarkReadThrough_Hit(b *testing.B) {
	storage, err := lru.New(128)
	if err != nil {
		b.Fatal(err)
	}
	g := cache.NewGenerations(cache.New(storage))
	ctx := b.Context()
	fill := func(context.Context) (item, error) { return item{Name: "v"}, nil }
	_, _ = cache.ReadThrough(ctx, g, "kind", "get:a", fill)
	for b.Loop() {
		_, _ = cache.ReadThrough(ctx, g, "kind", "get:a", fill)
	}
}
