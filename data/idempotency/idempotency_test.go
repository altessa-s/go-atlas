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
	require.Nil(t, state, "expected nil state for new lock")
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

	_, _, _ = k.AttemptLock(ctx, "key1")
	_ = k.Complete(ctx, "key1", map[string]string{"result": "ok"})

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

	_, _, _ = k.AttemptLock(ctx, "key1")
	err := k.Complete(ctx, "key1", "result-data")
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
		AttemptLockFunc: func(_ context.Context, _ string, _ []byte) (bool, []byte, error) {
			called = true
			return true, nil, nil
		},
		CompleteFunc: func(_ context.Context, _ string, _ []byte) error { return nil },
		DeleteFunc:   func(_ context.Context, _ string) error { return nil },
	}

	_, _, _ = sf.AttemptLock(t.Context(), "key", nil)
	require.True(t, called, "AttemptLockFunc not called")
	_ = sf.Complete(t.Context(), "key", nil)
	_ = sf.Delete(t.Context(), "key")
}
