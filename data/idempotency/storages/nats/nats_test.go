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

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed on new key")
	require.Nil(t, existingVal)
}

func TestStorage_AttemptLock_Existing(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("val1"))
	require.NoError(t, err)
	require.True(t, locked, "expected first lock to succeed")

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
	require.NoError(t, err)
	require.False(t, locked, "expected second lock to fail")
	require.Equal(t, "val1", string(existingVal))
}

func TestStorage_AttemptLock_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, existingVal, err := storage.AttemptLock(ctx, "", []byte("val"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
	require.False(t, locked)
	require.Nil(t, existingVal)
}

func TestStorage_Complete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("lock-val"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	require.NoError(t, storage.Complete(ctx, "key1", []byte("complete-val")))

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("new-val"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail after completion")
	require.Equal(t, "complete-val", string(existingVal))
}

func TestStorage_Complete_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	require.ErrorIs(t, storage.Complete(ctx, "", []byte("val")), storages.ErrEmptyKey)
}

func TestStorage_Delete(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	locked, _, err := storage.AttemptLock(ctx, "key1", []byte("val"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	require.NoError(t, storage.Delete(ctx, "key1"))

	locked, existingVal, err := storage.AttemptLock(ctx, "key1", []byte("val2"))
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
	locked, _, err := storage.AttemptLock(ctx, "lifecycle", []byte("in-progress"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed")

	// Complete
	require.NoError(t, storage.Complete(ctx, "lifecycle", []byte("done")))

	// Verify completed value
	locked, val, err := storage.AttemptLock(ctx, "lifecycle", []byte("retry"))
	require.NoError(t, err)
	require.False(t, locked, "expected lock to fail")
	require.Equal(t, "done", string(val))

	// Delete
	require.NoError(t, storage.Delete(ctx, "lifecycle"))

	// Should be able to lock again
	locked, _, err = storage.AttemptLock(ctx, "lifecycle", []byte("new"))
	require.NoError(t, err)
	require.True(t, locked, "expected lock to succeed after deletion")
}
