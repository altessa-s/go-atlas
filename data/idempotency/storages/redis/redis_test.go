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
	if storage == nil {
		t.Fatal("expected non-nil storage")
	}
}

func TestStorage_AttemptLock_New(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	locked, existingVal, err := storage.AttemptLock(ctx, key, val)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed on new key")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value, got: %v", existingVal)
	}
}

func TestStorage_AttemptLock_Existing(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val1 := []byte("first-value")
	val2 := []byte("second-value")

	// First lock should succeed
	locked, _, err := storage.AttemptLock(ctx, key, val1)
	if err != nil {
		t.Fatalf("unexpected error on first lock: %v", err)
	}
	if !locked {
		t.Error("expected first lock to succeed")
	}

	// Second lock should fail and return first value
	locked, existingVal, err := storage.AttemptLock(ctx, key, val2)
	if err != nil {
		t.Fatalf("unexpected error on second lock: %v", err)
	}
	if locked {
		t.Error("expected second lock to fail")
	}
	if string(existingVal) != string(val1) {
		t.Errorf("expected existing value %q, got %q", val1, existingVal)
	}
}

func TestStorage_Complete(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	lockVal := []byte("lock-value")
	completeVal := []byte("complete-value")

	// Lock the key
	locked, _, err := storage.AttemptLock(ctx, key, lockVal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	// Complete the key with new value
	if err := storage.Complete(ctx, key, completeVal); err != nil {
		t.Fatalf("unexpected error completing: %v", err)
	}

	// Attempt lock again should fail and return completed value
	locked, existingVal, err := storage.AttemptLock(ctx, key, []byte("new-value"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Error("expected lock to fail after completion")
	}
	if string(existingVal) != string(completeVal) {
		t.Errorf("expected completed value %q, got %q", completeVal, existingVal)
	}
}

func TestStorage_Delete(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	// Lock the key
	locked, _, err := storage.AttemptLock(ctx, key, val)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	// Delete the key
	if err := storage.Delete(ctx, key); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	// Lock should succeed again after deletion
	locked, existingVal, err := storage.AttemptLock(ctx, key, val)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed after deletion")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value after deletion, got: %v", existingVal)
	}
}

func TestStorage_EmptyKey(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	// AttemptLock with empty key should return (true, nil, nil)
	locked, existingVal, err := storage.AttemptLock(ctx, "", []byte("value"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !locked {
		t.Error("expected empty key to return locked=true")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value, got: %v", existingVal)
	}

	// Complete with empty key should return nil
	if err := storage.Complete(ctx, "", []byte("value")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Delete with empty key should return nil
	if err := storage.Delete(ctx, ""); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed")
	}

	// Fast forward past TTL
	mr.FastForward(2 * time.Second)

	// Lock should succeed again after expiry
	locked, existingVal, err := storage.AttemptLock(ctx, key, []byte("new-value"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed after TTL expiry")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value after expiry, got: %v", existingVal)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Fatal("expected lock to succeed with prefix1")
	}

	// Lock with second prefix should also succeed (different namespace)
	locked, existingVal, err := storage2.AttemptLock(ctx, key, val2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !locked {
		t.Error("expected lock to succeed with prefix2 (isolated namespace)")
	}
	if existingVal != nil {
		t.Errorf("expected nil existing value, got: %v", existingVal)
	}

	// Verify both values are stored independently
	locked, existingVal, err = storage1.AttemptLock(ctx, key, []byte("new"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Error("expected lock to fail on prefix1")
	}
	if string(existingVal) != string(val1) {
		t.Errorf("expected value %q on prefix1, got %q", val1, existingVal)
	}
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
	if count := successCount.Load(); count != 1 {
		t.Errorf("expected exactly 1 successful lock, got %d", count)
	}
}
