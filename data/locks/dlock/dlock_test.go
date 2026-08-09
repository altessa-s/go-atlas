// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock"
)

func TestNewWithNoop(t *testing.T) {
	dl := dlock.NewWithNoop()
	require.NotNil(t, dl, "NewWithNoop() returned nil")
}

func TestDLock_Lock_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	_, err := dl.Lock(t.Context(), "")
	require.Error(t, err, "Lock(\"\") should return error")
}

func TestDLock_Lock_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	lk, err := dl.Lock(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, lk, "Lock() returned nil")
	require.NoError(t, lk.Release(ctx))
}

func TestDLock_Synchronize_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	called := false
	err := dl.Synchronize(ctx, "test-key", func(ctx context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called, "fn was not called")
}

func TestDLock_Synchronize_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	err := dl.Synchronize(t.Context(), "", func(ctx context.Context) error {
		return nil
	})
	require.Error(t, err, "Synchronize(\"\") should return error")
}

func TestDLock_Synchronize_FnError(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	wantErr := errors.New("fn error")
	err := dl.Synchronize(ctx, "key", func(ctx context.Context) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
}

func TestDLock_GetLockInfo_EmptyKey(t *testing.T) {
	dl := dlock.NewWithNoop()
	_, err := dl.GetLockInfo(t.Context(), "")
	require.Error(t, err, "GetLockInfo(\"\") should return error")
}

func TestDLock_GetLockInfo_Success(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	info, err := dl.GetLockInfo(ctx, "test-key")
	require.NoError(t, err)
	require.Equal(t, "test-key", info.Key)
	require.Equal(t, uint64(0), info.FencingToken)
}

func TestDLock_Close(t *testing.T) {
	dl := dlock.NewWithNoop()
	require.NoError(t, dl.Close(t.Context()))
}

func TestDLock_Synchronize_PassesTheCallersContext(t *testing.T) {
	dl := dlock.NewWithNoop()
	ctx := t.Context()

	var seen context.Context
	err := dl.Synchronize(ctx, "key", func(inner context.Context) error {
		seen = inner
		return nil
	})
	require.NoError(t, err)

	// Synchronize must not narrow the caller's context on the way in: the same
	// context scopes the lock, and an acquisition deadline folded into it would
	// release the lock while fn was still running.
	deadline, hasDeadline := seen.Deadline()
	ctxDeadline, ctxHasDeadline := ctx.Deadline()
	require.Equal(t, ctxHasDeadline, hasDeadline, "Synchronize added a deadline of its own")
	if ctxHasDeadline {
		require.Equal(t, ctxDeadline, deadline)
	}
}
