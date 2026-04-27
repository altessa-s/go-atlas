// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestAttemptLock_New(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.True(t, ok, "expected lock acquired")
	require.NotNil(t, state, "AttemptLock now returns a non-nil State carrying the lock token on acquire")
	require.Equal(t, storages.StatusInProgress, state.Status)
	require.NotNil(t, state.LockToken(), "lock token must be populated for Complete to succeed")
}

func TestAttemptLock_InProgress(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "expected lock NOT acquired (already in progress)")
	require.NotNil(t, state)
	require.Equal(t, storages.StatusInProgress, state.Status)
}

func TestAttemptLock_Completed(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockState, _ := k.AttemptLock(ctx, "key1")
	_ = k.Complete(ctx, "key1", map[string]string{"result": "ok"}, lockState)

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "expected lock NOT acquired (completed)")
	require.NotNil(t, state)
	require.Equal(t, storages.StatusSuccess, state.Status)
}

func TestComplete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockState, _ := k.AttemptLock(ctx, "key1")
	err := k.Complete(ctx, "key1", "result-data", lockState)
	require.NoError(t, err)
}

func TestDelete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")
	err := k.Delete(ctx, "key1")
	require.NoError(t, err)

	// After delete, lock should succeed again.
	ok, _, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.True(t, ok, "expected lock after delete")
}

func TestAttemptLock_EmptyKey(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)

	ok, state, err := k.AttemptLock(t.Context(), "")
	require.ErrorIs(t, err, ErrEmptyKey,
		"empty key must surface ErrEmptyKey instead of silently succeeding (which would disable dedupe)")
	require.False(t, ok)
	require.Nil(t, state)
}

func TestStorageFunc(t *testing.T) {
	called := false
	sf := StorageFunc{
		AttemptLockFunc: func(_ context.Context, _ string, _ []byte) (bool, []byte, []byte, error) {
			called = true
			return true, nil, nil, nil
		},
		CompleteFunc: func(_ context.Context, _ string, _ []byte, _ []byte) error { return nil },
		DeleteFunc:   func(_ context.Context, _ string) error { return nil },
	}

	_, _, _, _ = sf.AttemptLock(t.Context(), "key", nil)
	require.True(t, called, "AttemptLockFunc not called")
	_ = sf.Complete(t.Context(), "key", nil, nil)
	_ = sf.Delete(t.Context(), "key")
}

// TestComplete_NilLockState verifies that Complete refuses to write
// without the *State returned by AttemptLock — passing nil silently
// would defeat the stolen-lock guard.
func TestComplete_NilLockState(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)

	err := k.Complete(t.Context(), "key", "data", nil)
	require.ErrorIs(t, err, ErrMissingLockState)
}

// TestComplete_StolenLock verifies that ErrLockStolen from the
// underlying Storage propagates unchanged through the Keeper layer.
func TestComplete_StolenLock(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockStateA, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.NotNil(t, lockStateA)

	// Simulate B taking over the key after A's TTL expired.
	require.NoError(t, k.Delete(ctx, "key1"))
	_, _, err = k.AttemptLock(ctx, "key1")
	require.NoError(t, err)

	err = k.Complete(ctx, "key1", "result-from-A", lockStateA)
	require.ErrorIs(t, err, ErrLockStolen,
		"stale Complete must surface ErrLockStolen rather than overwriting the new holder")
}
