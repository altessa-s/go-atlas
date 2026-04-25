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

func TestMemory_Complete_NonExistent(t *testing.T) {
	s := New()
	ctx := t.Context()

	// Complete upserts on nonexistent key
	err := s.Complete(ctx, "nonexistent", []byte("val"))
	require.NoError(t, err)
	// Key should now exist
	ok, existing, _ := s.AttemptLock(ctx, "nonexistent", []byte("new"))
	require.False(t, ok, "expected lock NOT acquired after Complete upsert")
	require.Equal(t, "val", string(existing))
}

func TestMemory_Complete_EmptyKey(t *testing.T) {
	s := New()
	err := s.Complete(t.Context(), "", []byte("val"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}

func TestMemory_AttemptLock_EmptyKey(t *testing.T) {
	s := New()
	ok, existing, err := s.AttemptLock(t.Context(), "", []byte("val"))
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
	s := New(WithTtl(10 * time.Millisecond))
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "k1", []byte("v1"))
	_, _, _ = s.AttemptLock(ctx, "k2", []byte("v2"))

	time.Sleep(20 * time.Millisecond)

	s.RunCleanup()

	// Both keys should be cleaned up, so new locks should succeed
	ok1, _, _ := s.AttemptLock(ctx, "k1", []byte("new"))
	ok2, _, _ := s.AttemptLock(ctx, "k2", []byte("new"))
	require.True(t, ok1, "expected lock k1 to succeed after cleanup")
	require.True(t, ok2, "expected lock k2 to succeed after cleanup")
}

func TestMemory_RunCleanup_NoTTL(t *testing.T) {
	s := New(WithTtl(0))
	// Should not panic
	s.RunCleanup()
}

func TestMemory_RunCleanup_SchedulerManaged(t *testing.T) {
	s := New()
	s.schedulerCleanupRegistered.Store(true)

	// Should return immediately without cleaning
	s.RunCleanup()
}

func TestMemory_Complete_Expired(t *testing.T) {
	s := New(WithTtl(10 * time.Millisecond))
	ctx := t.Context()

	_, _, _ = s.AttemptLock(ctx, "k1", []byte("v1"))
	time.Sleep(20 * time.Millisecond)

	// After expiry, the entry is still in map but AttemptLock treats it as new.
	// Complete on expired key should still work (upsert behavior).
	err := s.Complete(ctx, "k1", []byte("done"))
	require.NoError(t, err)
}
