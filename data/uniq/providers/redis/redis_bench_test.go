// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	uniqredis "github.com/altessa-s/go-atlas/data/uniq/providers/redis"
	goredis "github.com/redis/go-redis/v9"
)

func benchProvider(b *testing.B) *uniqredis.Provider {
	b.Helper()
	mr := miniredis.RunT(b)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { client.Close() })
	return uniqredis.New(client)
}

func BenchmarkProvider_Add(b *testing.B) {
	p := benchProvider(b)
	ctx := b.Context()
	for b.Loop() {
		_ = p.Add(ctx, "bench-key")
	}
}

func BenchmarkProvider_Exist(b *testing.B) {
	p := benchProvider(b)
	ctx := b.Context()
	_ = p.Add(ctx, "bench-key")
	for b.Loop() {
		_, _ = p.Exist(ctx, "bench-key")
	}
}

func BenchmarkProvider_GetValue(b *testing.B) {
	p := benchProvider(b)
	ctx := b.Context()
	_ = p.AddWithValue(ctx, "bench-key", []byte("data"))
	for b.Loop() {
		_, _ = p.GetValue(ctx, "bench-key")
	}
}
