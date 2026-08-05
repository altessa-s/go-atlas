// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
)

func TestMemory_AttemptLock_New(t *testing.T) {
	s := New()
	ctx := t.Context()

	ok, existing, _, err := s.AttemptLock(ctx, "key1", []byte("val"))
	require.NoError(t, err)
	require.True(t, ok, "expected lock acquired")
	require.Nil(t, existing)
}

func TestMemory_AttemptLock_Existing(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _, _ = s.AttemptLock(ctx, "key1", []byte("first"))

	ok, existing, _, err := s.AttemptLock(ctx, "key1", []byte("second"))
	require.NoError(t, err)
	require.False(t, ok, "expected lock NOT acquired")
	require.Equal(t, "first", string(existing))
}

func TestMemory_Complete(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, lockToken, _ := s.AttemptLock(ctx, "key1", []byte("in-progress"))
	err := s.Complete(ctx, "key1", []byte("done"), lockToken)
	require.NoError(t, err)

	// Attempting lock should now return the completed value.
	ok, existing, _, _ := s.AttemptLock(ctx, "key1", []byte("new"))
	require.False(t, ok, "expected lock NOT acquired after complete")
	require.Equal(t, "done", string(existing))
}

func TestMemory_Delete(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _, _ = s.AttemptLock(ctx, "key1", []byte("val"))
	err := s.Delete(ctx, "key1")
	require.NoError(t, err)

	// After delete, lock should succeed.
	ok, _, _, _ := s.AttemptLock(ctx, "key1", []byte("val2"))
	require.True(t, ok, "expected lock acquired after delete")
}

func TestMemory_TTLExpiry(t *testing.T) {
	s := New(WithTTL(10 * time.Millisecond))
	ctx := t.Context()

	_, _, _, _ = s.AttemptLock(ctx, "key1", []byte("val"))
	time.Sleep(20 * time.Millisecond)

	// Expired entry should be treated as non-existent.
	ok, _, _, err := s.AttemptLock(ctx, "key1", []byte("val2"))
	require.NoError(t, err)
	require.True(t, ok, "expected lock acquired after TTL expiry")
}

func TestMemory_Concurrent(t *testing.T) {
	s := New()
	ctx := t.Context()
	var wg sync.WaitGroup
	n := 100
	acquired := make(chan bool, n)

	for range n {
		wg.Go(func() {
			ok, _, _, _ := s.AttemptLock(ctx, "shared-key", []byte("val"))
			acquired <- ok
		})
	}
	wg.Wait()
	close(acquired)

	count := 0
	for ok := range acquired {
		if ok {
			count++
		}
	}
	require.Equal(t, 1, count, "expected exactly 1 lock acquired")
}

// TestMemory_AttemptLockWithTTL_OverridesDefault verifies per-call
// TTL on AttemptLock: a short lockTtl wins over the long default,
// and the entry is cleaned up by RunCleanup after expiry.
func TestMemory_AttemptLockWithTTL_OverridesDefault(t *testing.T) {
	s := New(WithTTL(time.Hour)) // long default
	ctx := t.Context()

	ok, _, _, err := s.AttemptLockWithTTL(ctx, "k", []byte("lock"), 10*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(20 * time.Millisecond)
	s.RunCleanup()

	// Re-lock should succeed because the per-call TTL fired.
	ok, _, _, err = s.AttemptLockWithTTL(ctx, "k", []byte("lock2"), 0)
	require.NoError(t, err)
	require.True(t, ok, "per-call lockTtl must override default; key should have expired")
}

// TestMemory_Steal_Success verifies that Steal replaces the value
// when current bytes match expectedVal and returns a fresh lock token.
func TestMemory_Steal_Success(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _, err := s.AttemptLock(ctx, "k", []byte("orphan"))
	require.NoError(t, err)

	newToken, err := s.Steal(ctx, "k", []byte("orphan"), []byte("fresh"))
	require.NoError(t, err)
	require.NotNil(t, newToken)
	require.Equal(t, "fresh", string(newToken),
		"memory backend's lock token is a copy of the stored value")

	// The original "orphan" token must no longer satisfy Complete.
	err = s.Complete(ctx, "k", []byte("done"), []byte("orphan"))
	require.ErrorIs(t, err, storages.ErrLockStolen)

	// The stolen token must succeed.
	require.NoError(t, s.Complete(ctx, "k", []byte("done"), newToken))
}

// TestMemory_Steal_Mismatch verifies that Steal surfaces ErrLockStolen
// when the current value differs from expectedVal.
func TestMemory_Steal_Mismatch(t *testing.T) {
	s := New()
	ctx := t.Context()

	_, _, _, err := s.AttemptLock(ctx, "k", []byte("current"))
	require.NoError(t, err)

	_, err = s.Steal(ctx, "k", []byte("stale"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen,
		"Steal must reject when expectedVal does not match current bytes")

	_, existing, _, _ := s.AttemptLock(ctx, "k", []byte("retry"))
	require.Equal(t, "current", string(existing),
		"failed Steal must not modify the stored value")
}

// TestMemory_Steal_KeyMissing verifies that Steal on an absent key
// surfaces ErrLockStolen.
func TestMemory_Steal_KeyMissing(t *testing.T) {
	s := New()
	_, err := s.Steal(t.Context(), "nope", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
}

// TestMemory_Steal_EmptyKey verifies the empty-key contract.
func TestMemory_Steal_EmptyKey(t *testing.T) {
	s := New()
	_, err := s.Steal(t.Context(), "", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}

// TestMemory_Complete_StolenLock is the regression test for the
// stolen-lock CAS guard. Sequence:
//
//  1. Holder A AttemptLock with val "a-lock" → wins, gets tokenA.
//  2. We force the lock free (Delete) and B re-locks with "b-lock".
//  3. A calls Complete with the stale tokenA → must fail with
//     ErrLockStolen and must NOT overwrite B's value.
func TestMemory_Complete_StolenLock(t *testing.T) {
	s := New()
	ctx := t.Context()

	okA, _, tokenA, err := s.AttemptLock(ctx, "k", []byte("a-lock"))
	require.NoError(t, err)
	require.True(t, okA)
	require.NotNil(t, tokenA)

	// Lock TTL would have expired in the real world; force-delete and
	// let another holder take it over.
	require.NoError(t, s.Delete(ctx, "k"))
	okB, _, _, err := s.AttemptLock(ctx, "k", []byte("b-lock"))
	require.NoError(t, err)
	require.True(t, okB)

	// Stale Complete from A must be rejected.
	err = s.Complete(ctx, "k", []byte("a-result"), tokenA)
	require.ErrorIs(t, err, storages.ErrLockStolen)

	// B's claim survives untouched.
	_, existing, _, _ := s.AttemptLock(ctx, "k", []byte("c-lock"))
	require.Equal(t, "b-lock", string(existing),
		"stolen Complete must not overwrite the new holder's value")
}
