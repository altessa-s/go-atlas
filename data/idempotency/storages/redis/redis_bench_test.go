// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

func benchSetup(b *testing.B) (*idempredis.Storage, *miniredis.Miniredis) {
	b.Helper()
	client, mr := testhelpers.RedisClient(b)
	return idempredis.New(client), mr
}

func BenchmarkStorage_AttemptLock(b *testing.B) {
	storage, mr := benchSetup(b)
	ctx := b.Context()

	val := []byte("benchmark-value")

	b.ResetTimer()
	for b.Loop() {
		// Use unique key for each iteration to avoid conflicts
		key := "bench-key"
		mr.FlushAll() // Clear between iterations

		_, _, _, err := storage.AttemptLock(ctx, key, val)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

func BenchmarkStorage_Complete(b *testing.B) {
	storage, mr := benchSetup(b)
	ctx := b.Context()

	key := "bench-key"
	lockVal := []byte("lock-value")
	completeVal := []byte("complete-value")

	b.ResetTimer()
	for b.Loop() {
		mr.FlushAll() // Clear between iterations

		// Lock first
		_, _, lockToken, err := storage.AttemptLock(ctx, key, lockVal)
		if err != nil {
			b.Fatalf("unexpected error locking: %v", err)
		}

		// Benchmark the complete operation
		err = storage.Complete(ctx, key, completeVal, lockToken)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}
