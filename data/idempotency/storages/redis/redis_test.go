// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	idempredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// setupStorage creates a new Storage instance backed by miniredis for testing.
func setupStorage(tb testing.TB) (*idempredis.Storage, *miniredis.Miniredis) {
	tb.Helper()

	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	storage := idempredis.New(client)
	return storage, mr
}

func TestNew(t *testing.T) {
	storage, _ := setupStorage(t)
	require.NotNil(t, storage)
}

func TestStorage_AttemptLock_New(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	locked, existingVal, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed on new key")
	require.Nil(t, existingVal)
}

func TestStorage_AttemptLock_Existing(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val1 := []byte("first-value")
	val2 := []byte("second-value")

	// First lock should succeed
	locked, _, err := storage.AttemptLock(ctx, key, val1)
	require.NoError(t, err)
	require.True(t, locked, "expected first lock to succeed")

	// Second lock should fail and return first value
	locked, existingVal, err := storage.AttemptLock(ctx, key, val2)
	require.NoError(t, err)
	require.False(t, locked, "expected second lock to fail")
	require.Equal(t, string(val1), string(existingVal))
}

func TestStorage_Complete(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	lockVal := []byte("lock-value")
	completeVal := []byte("complete-value")

	// Lock the key
	locked, _, err := storage.AttemptLock(ctx, key, lockVal)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Complete the key with new value
	require.NoError(t, storage.Complete(ctx, key, completeVal))

	// Attempt lock again should fail and return completed value
	locked, existingVal, err := storage.AttemptLock(ctx, key, []byte("new-value"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail after completion")
	require.Equal(t, string(completeVal), string(existingVal))
}

func TestStorage_Delete(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	// Lock the key
	locked, _, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Delete the key
	require.NoError(t, storage.Delete(ctx, key))

	// Lock should succeed again after deletion
	locked, existingVal, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after deletion")
	require.Nil(t, existingVal)
}

func TestStorage_EmptyKey(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	// AttemptLock with empty key should return (true, nil, nil)
	locked, existingVal, err := storage.AttemptLock(ctx, "", []byte("value"))
	require.NoError(t, err)
	require.True(t, locked, "expected empty key to return locked=true")
	require.Nil(t, existingVal)

	// Complete with empty key should return nil
	require.NoError(t, storage.Complete(ctx, "", []byte("value")))

	// Delete with empty key should return nil
	require.NoError(t, storage.Delete(ctx, ""))
}

func TestStorage_TTLExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	// Create storage with short TTL
	storage := idempredis.New(client, idempredis.WithTtl(1*time.Second))
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	// Lock the key
	locked, _, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Fast forward past TTL
	mr.FastForward(2 * time.Second)

	// Lock should succeed again after expiry
	locked, existingVal, err := storage.AttemptLock(ctx, key, []byte("new-value"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after TTL expiry")
	require.Nil(t, existingVal)
}

func TestStorage_WithKeyPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	storage1 := idempredis.New(client, idempredis.WithKeyPrefix("prefix1:"))
	storage2 := idempredis.New(client, idempredis.WithKeyPrefix("prefix2:"))
	ctx := t.Context()

	key := "test-key"
	val1 := []byte("value1")
	val2 := []byte("value2")

	// Lock with first prefix
	locked, _, err := storage1.AttemptLock(ctx, key, val1)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed with prefix1")

	// Lock with second prefix should also succeed (different namespace)
	locked, existingVal, err := storage2.AttemptLock(ctx, key, val2)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed with prefix2 (isolated namespace)")
	require.Nil(t, existingVal)

	// Verify both values are stored independently
	locked, existingVal, err = storage1.AttemptLock(ctx, key, []byte("new"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail on prefix1")
	require.Equal(t, string(val1), string(existingVal))
}

func TestStorage_Concurrent(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "concurrent-key"
	numGoroutines := 100

	var successCount atomic.Int32
	var wg sync.WaitGroup

	// Launch concurrent lock attempts
	for i := range numGoroutines {
		wg.Go(func() {
			val := []byte("goroutine-value")
			locked, _, err := storage.AttemptLock(ctx, key, val)
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
				return
			}
			if locked {
				successCount.Add(1)
			}
		})
	}

	wg.Wait()

	// Exactly one goroutine should have succeeded
	require.Equal(t, int32(1), successCount.Load(), "expected exactly 1 successful lock")
}
