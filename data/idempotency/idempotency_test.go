// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestAttemptLock_New(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	ok, state, err := k.AttemptLock(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected lock acquired")
	}
	if state != nil {
		t.Error("expected nil state for new lock")
	}
}

func TestAttemptLock_InProgress(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")

	ok, state, err := k.AttemptLock(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected lock NOT acquired (already in progress)")
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Status != storages.StatusInProgress {
		t.Errorf("status = %s, want %s", state.Status, storages.StatusInProgress)
	}
}

func TestAttemptLock_Completed(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")
	_ = k.Complete(ctx, "key1", map[string]string{"result": "ok"})

	ok, state, err := k.AttemptLock(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected lock NOT acquired (completed)")
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Status != storages.StatusSuccess {
		t.Errorf("status = %s, want %s", state.Status, storages.StatusSuccess)
	}
}

func TestComplete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")
	err := k.Complete(ctx, "key1", "result-data")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")
	err := k.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// After delete, lock should succeed again.
	ok, _, err := k.AttemptLock(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected lock after delete")
	}
}

func TestAttemptLock_EmptyKey(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)

	ok, state, err := k.AttemptLock(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected true for empty key")
	}
	if state != nil {
		t.Error("expected nil state")
	}
}

func TestStorageFunc(t *testing.T) {
	called := false
	sf := StorageFunc{
		AttemptLockFunc: func(_ context.Context, _ string, _ []byte) (bool, []byte, error) {
			called = true
			return true, nil, nil
		},
		CompleteFunc: func(_ context.Context, _ string, _ []byte) error { return nil },
		DeleteFunc:   func(_ context.Context, _ string) error { return nil },
	}

	_, _, _ = sf.AttemptLock(t.Context(), "key", nil)
	if !called {
		t.Error("AttemptLockFunc not called")
	}
	_ = sf.Complete(t.Context(), "key", nil)
	_ = sf.Delete(t.Context(), "key")
}
