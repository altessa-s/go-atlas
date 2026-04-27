// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

func setupStorage(tb testing.TB) *idempnats.Storage {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	_, js := testhelpers.ConnectJetStream(tb, ns)

	storage, err := idempnats.New(js, idempnats.WithBucket(tb.Name()))
	require.NoError(tb, err)
	return storage
}

func TestNew(t *testing.T) {
	storage := setupStorage(t)
	require.NotNil(t, storage)
}

func TestNew_NilJetStream(t *testing.T) {
	_, err := idempnats.New(nil)
	require.Error(t, err)
}

func TestStorage_AttemptLock_New(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, existingVal, _, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed on new key")
	require.Nil(t, existingVal)
}

func TestStorage_AttemptLock_Existing(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, _, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	require.NoError(t, err)
	require.True(t, locked, "expected first lock to succeed")

	locked, existingVal, _, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
	require.NoError(t, err)
	require.False(t, locked, "expected second lock to fail")
	require.Equal(t, "val1", string(existingVal))
}

func TestStorage_AttemptLock_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, existingVal, _, err := storage.AttemptLock(ctx, "", []byte("val"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
	require.False(t, locked)
	require.Nil(t, existingVal)
}

func TestStorage_Complete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, lockToken, err := storage.AttemptLock(ctx, "key1", []byte("lock-val"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	require.NoError(t, storage.Complete(ctx, "key1", []byte("complete-val"), lockToken))

	locked, existingVal, _, err := storage.AttemptLock(ctx, "key1", []byte("new-val"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail after completion")
	require.Equal(t, "complete-val", string(existingVal))
}

func TestStorage_Complete_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	require.ErrorIs(t, storage.Complete(ctx, "", []byte("val"), nil), storages.ErrEmptyKey)
}

func TestStorage_Delete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, _, err := storage.AttemptLock(ctx, "key1", []byte("val"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	require.NoError(t, storage.Delete(ctx, "key1"))

	locked, existingVal, _, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after deletion")
	require.Nil(t, existingVal)
}

func TestStorage_Delete_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	require.ErrorIs(t, storage.Delete(ctx, ""), storages.ErrEmptyKey)
}

func TestStorage_FullLifecycle(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	// Lock
	locked, _, lockToken, err := storage.AttemptLock(ctx, "lifecycle", []byte("in-progress"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Complete
	require.NoError(t, storage.Complete(ctx, "lifecycle", []byte("done"), lockToken))

	// Verify completed value
	locked, val, _, err := storage.AttemptLock(ctx, "lifecycle", []byte("retry"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail")
	require.Equal(t, "done", string(val))

	// Delete
	require.NoError(t, storage.Delete(ctx, "lifecycle"))

	// Should be able to lock again
	locked, _, _, err = storage.AttemptLock(ctx, "lifecycle", []byte("new"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after deletion")
}

// TestStorage_Complete_StolenLock is the regression test for the
// stolen-lock CAS guard backed by [jetstream.KV.Update] with a fixed
// expected revision. Sequence:
//
//  1. Holder A AttemptLock with val "a-lock" → wins, gets revision-encoded tokenA.
//  2. Force-delete and let B re-lock with "b-lock" — bucket revision advances.
//  3. A calls Complete with the stale revision token → must fail with
//     ErrLockStolen and must NOT overwrite B's value.
func TestStorage_Complete_StolenLock(t *testing.T) {
	storage := setupStorage(t)
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

// TestStorage_Complete_StolenLock_MalformedToken covers defensive
// handling of a token that doesn't have the expected 8-byte
// big-endian shape — surfaced as ErrLockStolen rather than a
// confusing transport error.
func TestStorage_Complete_StolenLock_MalformedToken(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	okA, _, _, err := storage.AttemptLock(ctx, "k", []byte("a-lock"))
	require.NoError(t, err)
	require.True(t, okA)

	err = storage.Complete(ctx, "k", []byte("a-result"), []byte("garbage"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
}
