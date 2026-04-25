// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

func BenchmarkGetBytes(b *testing.B) {
	client, mr := setupRedis(b)
	defer mr.Close()
	ctx := b.Context()
	mr.Set("bench-key", "bench-value")
	notFound := errors.New("not found")

	for b.Loop() {
		_, _ = redisutils.GetBytes(ctx, client, "bench-key", notFound)
	}
}

func BenchmarkSetBytes(b *testing.B) {
	client, mr := setupRedis(b)
	defer mr.Close()
	ctx := b.Context()
	val := []byte("bench-value")

	for b.Loop() {
		_ = redisutils.SetBytes(ctx, client, "bench-key", val, 0)
	}
}

func BenchmarkDel(b *testing.B) {
	client, mr := setupRedis(b)
	defer mr.Close()
	ctx := b.Context()

	for b.Loop() {
		_ = redisutils.Del(ctx, client, "bench-key")
	}
}
