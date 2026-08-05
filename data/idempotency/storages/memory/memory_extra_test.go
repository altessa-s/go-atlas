// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
)

func TestMemory_Close(t *testing.T) {
	s := New()
	require.NoError(t, s.Close())
}

// TestMemory_Complete_NonExistent verifies the new CAS semantics:
// Complete on a missing key returns ErrLockStolen (the lock was either
// never acquired or has been deleted/expired since AttemptLock).
func TestMemory_Complete_NonExistent(t *testing.T) {
	s := New()
	ctx := t.Context()

	err := s.Complete(ctx, "nonexistent", []byte("val"), nil)
	require.ErrorIs(t, err, storages.ErrLockStolen)
}

func TestMemory_Complete_EmptyKey(t *testing.T) {
	s := New()
	err := s.Complete(t.Context(), "", []byte("val"), nil)
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}

func TestMemory_AttemptLock_EmptyKey(t *testing.T) {
	s := New()
	ok, existing, _, err := s.AttemptLock(t.Context(), "", []byte("val"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
	require.False(t, ok)
	require.Nil(t, existing)
}

func TestMemory_Delete_EmptyKey(t *testing.T) {
	s := New()
	err := s.Delete(t.Context(), "")
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}

func TestMemory_RunCleanup(t *testing.T) {
	s := New(WithTTL(10 * time.Millisecond))
	ctx := t.Context()

	_, _, _, _ = s.AttemptLock(ctx, "k1", []byte("v1"))
	_, _, _, _ = s.AttemptLock(ctx, "k2", []byte("v2"))

	time.Sleep(20 * time.Millisecond)

	s.RunCleanup()

	// Both keys should be cleaned up, so new locks should succeed
	ok1, _, _, _ := s.AttemptLock(ctx, "k1", []byte("new"))
	ok2, _, _, _ := s.AttemptLock(ctx, "k2", []byte("new"))
	require.True(t, ok1, "expected lock k1 to succeed after cleanup")
	require.True(t, ok2, "expected lock k2 to succeed after cleanup")
}

func TestMemory_RunCleanup_NoTTL(t *testing.T) {
	s := New(WithTTL(0))
	// Should not panic
	s.RunCleanup()
}

func TestMemory_RunCleanup_SchedulerManaged(t *testing.T) {
	s := New()
	s.cleanupTask.MarkRegistered()

	// Should return immediately without cleaning
	s.RunCleanup()
}

// TestMemory_RunCleanup_KeepsRefreshedEntry pins the cleanup predicate
// against entries whose TTL was re-armed before the sweep: only entries
// expired at sweep start may be removed. The refresh-between-phases race
// itself (entry re-armed after the phase-1 collect must survive the phase-2
// delete) is pinned deterministically in data/internal/memcleanup, which
// this storage's cleanup delegates to.
func TestMemory_RunCleanup_KeepsRefreshedEntry(t *testing.T) {
	s := New(WithTTL(50 * time.Millisecond))
	ctx := t.Context()

	_, _, _, _ = s.AttemptLock(ctx, "stale", []byte("v1"))
	_, _, token, _ := s.AttemptLock(ctx, "refreshed", []byte("v2"))

	time.Sleep(100 * time.Millisecond)

	// Re-arm the TTL of "refreshed" via the production path (Complete
	// resets expiresAt), then sweep.
	require.NoError(t, s.Complete(ctx, "refreshed", []byte("done"), token))
	s.RunCleanup()

	// "stale" was expired and must be gone; "refreshed" must survive.
	ok, _, _, _ := s.AttemptLock(ctx, "stale", []byte("new"))
	require.True(t, ok, "expired entry must be removed by cleanup")
	ok, existing, _, _ := s.AttemptLock(ctx, "refreshed", []byte("new"))
	require.False(t, ok, "refreshed entry must survive cleanup")
	require.Equal(t, []byte("done"), existing)
}

// TestMemory_Complete_Expired verifies CAS semantics on expired keys:
// once the entry's TTL fires, Complete with the original lockToken
// fails with ErrLockStolen rather than upserting silently.
func TestMemory_Complete_Expired(t *testing.T) {
	s := New(WithTTL(10 * time.Millisecond))
	ctx := t.Context()

	_, _, lockToken, _ := s.AttemptLock(ctx, "k1", []byte("v1"))
	time.Sleep(20 * time.Millisecond)

	// Force the cleanup so the entry is actually removed (TTL expired
	// but cleanup may not have run yet). After RunCleanup the entry is
	// gone and Complete can no longer find a matching lock.
	s.RunCleanup()

	err := s.Complete(ctx, "k1", []byte("done"), lockToken)
	require.ErrorIs(t, err, storages.ErrLockStolen)
}
