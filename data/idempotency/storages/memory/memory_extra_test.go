// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"testing"
	"time"
)

func TestMemory_Close(t *testing.T) {
	s := New()
	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMemory_Complete_NonExistent(t *testing.T) {
	s := New()
	ctx := t.Context()

	// Complete upserts on nonexistent key
	err := s.Complete(ctx, "nonexistent", []byte("val"))
	if err != nil {
		t.Errorf("Complete() error = %v", err)
	}
	// Key should now exist
	ok, existing, _ := s.AttemptLock(ctx, "nonexistent", []byte("new"))
	if ok {
		t.Error("expected lock NOT acquired after Complete upsert")
	}
	if string(existing) != "val" {
		t.Errorf("existing = %q, want %q", existing, "val")
	}
}

func TestMemory_Complete_EmptyKey(t *testing.T) {
	s := New()
	err := s.Complete(t.Context(), "", []byte("val"))
	if err != nil {
		t.Errorf("Complete('') error = %v", err)
	}
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
	if !ok1 || !ok2 {
		t.Error("expected locks to succeed after cleanup of expired entries")
	}
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
	if err != nil {
		t.Errorf("Complete() on expired key error = %v", err)
	}
}
