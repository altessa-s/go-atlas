// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

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

// TestNew_ReplicasWiredToBucketConfig pins that WithReplicas reaches the
// jetstream.KeyValueConfig used for bucket creation. A capture double is used
// because a single-node test server cannot create buckets with replicas > 1.
func TestNew_ReplicasWiredToBucketConfig(t *testing.T) {
	t.Parallel()

	capture := &testhelpers.JetStreamKVCapture{}

	_, err := idempnats.New(capture,
		idempnats.WithBucket("idempotency-replicas"),
		idempnats.WithReplicas(3))
	require.NoError(t, err)

	require.Equal(t, "idempotency-replicas", capture.KVConfig.Bucket)
	require.Equal(t, 3, capture.KVConfig.Replicas)
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

// TestStorage_AttemptLockWithTTL_OverridesBucketTtl verifies per-key
// TTL on AttemptLock through jetstream.KeyTTL — the bucket-level TTL
// is long but the per-call TTL fires first.
func TestStorage_AttemptLockWithTTL_OverridesBucketTtl(t *testing.T) {
	t.Parallel()

	storage := setupStorage(t)
	ctx := t.Context()

	ok, _, _, err := storage.AttemptLockWithTTL(ctx, "k", []byte("lock"), time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// NATS expiry is lazy — give the server time to GC the entry.
	time.Sleep(2500 * time.Millisecond)

	ok, _, _, err = storage.AttemptLockWithTTL(ctx, "k", []byte("lock2"), 0)
	require.NoError(t, err)
	require.True(t, ok, "per-call lockTtl should have expired the entry well before bucket TTL")
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

// TestStorage_Steal_Success verifies the Get→bytes.Equal→Update CAS
// happy path: when current bytes match expectedVal, the value is
// replaced and a fresh revision-encoded token is returned.
func TestStorage_Steal_Success(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	_, _, origToken, err := storage.AttemptLock(ctx, "k", []byte("orphan"))
	require.NoError(t, err)
	require.NotNil(t, origToken)

	newToken, err := storage.Steal(ctx, "k", []byte("orphan"), []byte("fresh"))
	require.NoError(t, err)
	require.NotNil(t, newToken)
	require.NotEqual(t, origToken, newToken,
		"NATS Steal must return a fresh revision-encoded token")

	// Stale revision token must no longer satisfy Complete.
	err = storage.Complete(ctx, "k", []byte("done"), origToken)
	require.ErrorIs(t, err, storages.ErrLockStolen)
	require.NoError(t, storage.Complete(ctx, "k", []byte("done"), newToken))
}

// TestStorage_Steal_Mismatch verifies that Steal surfaces
// ErrLockStolen when current bytes differ from expectedVal.
func TestStorage_Steal_Mismatch(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	_, _, _, err := storage.AttemptLock(ctx, "k", []byte("current"))
	require.NoError(t, err)

	_, err = storage.Steal(ctx, "k", []byte("stale"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen)

	_, existing, _, _ := storage.AttemptLock(ctx, "k", []byte("retry"))
	require.Equal(t, "current", string(existing),
		"failed Steal must not modify the stored value")
}

// TestStorage_Steal_KeyMissing verifies the ErrKeyNotFound branch.
func TestStorage_Steal_KeyMissing(t *testing.T) {
	storage := setupStorage(t)
	_, err := storage.Steal(t.Context(), "nope", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrLockStolen)
}

// TestStorage_Steal_EmptyKey verifies the empty-key contract.
func TestStorage_Steal_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	_, err := storage.Steal(t.Context(), "", []byte("any"), []byte("fresh"))
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}
