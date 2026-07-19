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

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

// setupStorage creates a new Storage instance backed by miniredis for testing.
func setupStorage(tb testing.TB) (*idempredis.Storage, *miniredis.Miniredis) {
	tb.Helper()

	client, mr := testhelpers.RedisClient(tb)
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

	locked, existingVal, _, err := storage.AttemptLock(ctx, key, val)
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
	locked, _, _, err := storage.AttemptLock(ctx, key, val1)
	require.NoError(t, err)
	require.True(t, locked, "expected first lock to succeed")

	// Second lock should fail and return first value
	locked, existingVal, _, err := storage.AttemptLock(ctx, key, val2)
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
	locked, _, lockToken, err := storage.AttemptLock(ctx, key, lockVal)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Complete the key with new value
	require.NoError(t, storage.Complete(ctx, key, completeVal, lockToken))

	// Attempt lock again should fail and return completed value
	locked, existingVal, _, err := storage.AttemptLock(ctx, key, []byte("new-value"))
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
	locked, _, _, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Delete the key
	require.NoError(t, storage.Delete(ctx, key))

	// Lock should succeed again after deletion
	locked, existingVal, _, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after deletion")
	require.Nil(t, existingVal)
}

func TestStorage_EmptyKey(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	// Empty key on every method must surface ErrEmptyKey — silently
	// succeeding would let any caller bypass dedupe by sending a blank key.
	locked, existingVal, _, err := storage.AttemptLock(ctx, "", []byte("value"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
	require.False(t, locked)
	require.Nil(t, existingVal)

	require.ErrorIs(t, storage.Complete(ctx, "", []byte("value"), nil), storages.ErrEmptyKey)
	require.ErrorIs(t, storage.Delete(ctx, ""), storages.ErrEmptyKey)
}

func TestStorage_TTLExpiry(t *testing.T) {
	client, mr := testhelpers.RedisClient(t)

	// Create storage with short TTL
	storage := idempredis.New(client, idempredis.WithTtl(1*time.Second))
	ctx := t.Context()

	key := "test-key"
	val := []byte("test-value")

	// Lock the key
	locked, _, _, err := storage.AttemptLock(ctx, key, val)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Fast forward past TTL
	mr.FastForward(2 * time.Second)

	// Lock should succeed again after expiry
	locked, existingVal, _, err := storage.AttemptLock(ctx, key, []byte("new-value"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after TTL expiry")
	require.Nil(t, existingVal)
}

func TestStorage_WithKeyPrefix(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	storage1 := idempredis.New(client, idempredis.WithKeyPrefix("prefix1:"))
	storage2 := idempredis.New(client, idempredis.WithKeyPrefix("prefix2:"))
	ctx := t.Context()

	key := "test-key"
	val1 := []byte("value1")
	val2 := []byte("value2")

	// Lock with first prefix
	locked, _, _, err := storage1.AttemptLock(ctx, key, val1)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed with prefix1")

	// Lock with second prefix should also succeed (different namespace)
	locked, existingVal, _, err := storage2.AttemptLock(ctx, key, val2)
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed with prefix2 (isolated namespace)")
	require.Nil(t, existingVal)

	// Verify both values are stored independently
	locked, existingVal, _, err = storage1.AttemptLock(ctx, key, []byte("new"))
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
			locked, _, _, err := storage.AttemptLock(ctx, key, val)
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

// TestStorage_AttemptLockWithTTL_OverridesDefault verifies per-call
// TTL on AttemptLock via Redis's SET NX with explicit PX.
func TestStorage_AttemptLockWithTTL_OverridesDefault(t *testing.T) {
	storage, mr := setupStorage(t)
	ctx := t.Context()

	ok, _, _, err := storage.AttemptLockWithTTL(ctx, "k", []byte("lock"), 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	mr.FastForward(200 * time.Millisecond)

	ok, _, _, err = storage.AttemptLockWithTTL(ctx, "k", []byte("lock2"), 0)
	require.NoError(t, err)
	require.True(t, ok, "per-call lockTtl must shrink lifetime; key should have expired")
}

// TestStorage_Complete_StolenLock is the regression test for the
// stolen-lock CAS guard backed by the Lua script. Sequence:
//
//  1. Holder A AttemptLock with val "a-lock" → wins, gets tokenA.
//  2. Force-delete and let B re-lock with "b-lock".
//  3. A calls Complete with stale tokenA → must fail with
//     ErrLockStolen and must NOT overwrite B's value.
func TestStorage_Complete_StolenLock(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	okA, _, tokenA, err := storage.AttemptLock(ctx, "k", []byte("a-lock"))
	require.NoError(t, err)
	require.True(t, okA)
	require.NotNil(t, tokenA)

	require.NoError(t, storage.Delete(ctx, "k"))
	okB, _, _, err := storage.AttemptLock(ctx, "k", []byte("b-lock"))
	require.NoError(t, err)
	require.True(t, okB)

	err = storage.Complete(ctx, "k", []byte("a-result"), tokenA)
	require.ErrorIs(t, err, storages.ErrLockStolen)

	_, existing, _, _ := storage.AttemptLock(ctx, "k", []byte("c-lock"))
	require.Equal(t, "b-lock", string(existing),
		"stolen Complete must not overwrite the new holder's value")
}

// TestStorage_Complete_StolenLock_KeyExpired covers the second
// failure mode: AttemptLock succeeded, the TTL fired (Lua GET returns
// nil), and Complete must report ErrLockStolen rather than re-creating
// the key.
func TestStorage_Complete_StolenLock_KeyExpired(t *testing.T) {
	storage, mr := setupStorage(t)
	ctx := t.Context()

	okA, _, tokenA, err := storage.AttemptLock(ctx, "k", []byte("a-lock"))
	require.NoError(t, err)
	require.True(t, okA)

	mr.FastForward(25 * time.Hour) // beyond default 24h TTL

	err = storage.Complete(ctx, "k", []byte("a-result"), tokenA)
	require.ErrorIs(t, err, storages.ErrLockStolen)
}

// TestStorage_Steal_Success verifies the Lua-backed CAS-replace
// happy path: when current bytes match expectedVal, the value is
// rewritten and a fresh token is returned.
func TestStorage_Steal_Success(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	_, _, _, err := storage.AttemptLock(ctx, "k", []byte("orphan"))
	require.NoError(t, err)

	newToken, err := storage.Steal(ctx, "k", []byte("orphan"), []byte("fresh"))
	require.NoError(t, err)
	require.Equal(t, "fresh", string(newToken),
		"Redis backend's lock token mirrors the stored value")

	// Stale token must no longer satisfy Complete.
	err = storage.Complete(ctx, "k", []byte("done"), []byte("orphan"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
	require.NoError(t, storage.Complete(ctx, "k", []byte("done"), newToken))
}

// TestStorage_Steal_Mismatch verifies that Steal surfaces
// ErrLockStolen when current value differs from expectedVal, and that
// the failed call does not modify storage.
func TestStorage_Steal_Mismatch(t *testing.T) {
	storage, _ := setupStorage(t)
	ctx := t.Context()

	_, _, _, err := storage.AttemptLock(ctx, "k", []byte("current"))
	require.NoError(t, err)

	_, err = storage.Steal(ctx, "k", []byte("stale"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen)

	_, existing, _, _ := storage.AttemptLock(ctx, "k", []byte("retry"))
	require.Equal(t, "current", string(existing),
		"failed Steal must not modify the stored value")
}

// TestStorage_Steal_KeyMissing verifies the Lua "key not found"
// branch surfaces ErrLockStolen.
func TestStorage_Steal_KeyMissing(t *testing.T) {
	storage, _ := setupStorage(t)
	_, err := storage.Steal(t.Context(), "nope", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
}

// TestStorage_Steal_EmptyKey verifies the empty-key contract.
func TestStorage_Steal_EmptyKey(t *testing.T) {
	storage, _ := setupStorage(t)
	_, err := storage.Steal(t.Context(), "", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}
