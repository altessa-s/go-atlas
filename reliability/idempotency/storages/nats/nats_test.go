// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	idempnats "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

func setupStorage(tb testing.TB) *idempnats.Storage {
	tb.Helper()
	ns := natstest.StartNATSServer(tb)
	_, js := natstest.ConnectJetStream(tb, ns)

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

	capture := &bucketCapture{}

	_, err := idempnats.New(capture,
		idempnats.WithBucket("idempotency-replicas"),
		idempnats.WithReplicas(3))
	require.NoError(t, err)

	require.Equal(t, "idempotency-replicas", capture.KVConfig.Bucket)
	require.Equal(t, 3, capture.KVConfig.Replicas)
}

type bucketCapture struct {
	natstest.JetStreamKVCapture
}

func (c *bucketCapture) Stream(context.Context, string) (jetstream.Stream, error) {
	if c.KVConfig.Bucket == "" {
		return nil, jetstream.ErrStreamNotFound
	}
	return capturedStream{}, nil
}

type capturedStream struct {
	jetstream.Stream
}

func (capturedStream) CachedInfo() *jetstream.StreamInfo {
	return &jetstream.StreamInfo{Config: jetstream.StreamConfig{MaxMsgsPerSubject: 1, AllowMsgTTL: true}}
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

	ok, _, token, err := storage.AttemptLockWithTTL(ctx, "k", []byte("lock"), time.Second)
	require.NoError(t, err)
	require.True(t, ok)

	// NATS expiry is lazy — give the server time to GC the entry.
	time.Sleep(2500 * time.Millisecond)

	require.ErrorIs(t, storage.Complete(ctx, "k", []byte("late"), token), storages.ErrLockStolen)
	require.ErrorIs(t, storage.Release(ctx, "k", token), storages.ErrLockStolen)

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

func TestStorage_Steal_RefusesMatchingLiveLock(t *testing.T) {
	t.Parallel()
	store := setupStorage(t)
	ctx := t.Context()
	_, _, old, err := store.AttemptLock(ctx, "k", []byte("lock"))
	require.NoError(t, err)

	stolen, current, token, err := store.Steal(ctx, "k", []byte("lock"), []byte("replacement"), 0, time.Minute)
	require.NoError(t, err)
	require.False(t, stolen)
	require.Equal(t, []byte("lock"), current)
	require.Empty(t, token)
	require.NoError(t, store.Complete(ctx, "k", []byte("done"), old))
}

// TestStorage_Steal_Mismatch verifies that Steal returns the observed value.
func TestStorage_Steal_Mismatch(t *testing.T) {
	storage := setupStorage(t)
	ctx := t.Context()

	_, _, _, err := storage.AttemptLock(ctx, "k", []byte("current"))
	require.NoError(t, err)

	didSteal, current, _, err := storage.Steal(ctx, "k", []byte("stale"), []byte("fresh"), 0, 0)
	require.NoError(t, err)
	require.False(t, didSteal)
	require.Equal(t, []byte("current"), current)

	_, existing, _, _ := storage.AttemptLock(ctx, "k", []byte("retry"))
	require.Equal(t, "current", string(existing),
		"failed Steal must not modify the stored value")
}

// TestStorage_Steal_KeyMissing verifies the ErrKeyNotFound branch.
func TestStorage_Steal_KeyMissing(t *testing.T) {
	storage := setupStorage(t)
	didSteal, _, _, err := storage.Steal(t.Context(), "nope", []byte("any"), []byte("fresh"), 0, 0)
	require.NoError(t, err)
	require.False(t, didSteal)
}

// TestStorage_Steal_EmptyKey verifies the empty-key contract.
func TestStorage_Steal_EmptyKey(t *testing.T) {
	storage := setupStorage(t)
	_, _, _, err := storage.Steal(t.Context(), "", []byte("any"), []byte("fresh"), 0, 0)
	require.ErrorIs(t, err, storages.ErrEmptyKey)
}

func TestKeeperLeaseIgnoresReplicaClock(t *testing.T) {
	t.Parallel()
	store := setupStorage(t)
	ctx := t.Context()
	const key = "clock"
	storageKey := idempotency.ComposeKey("namespace", "", key)
	wire, err := json.Marshal(map[string]any{
		"status":    idempotency.StatusInProgress,
		"locked_at": time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	acquired, _, old, err := store.AttemptLock(ctx, storageKey, wire)
	require.NoError(t, err)
	require.True(t, acquired)

	k := idempotency.New(store, idempotency.WithMaxLockDuration(time.Second))
	acquired, state, err := k.AttemptLock(ctx, key)
	require.NoError(t, err)
	require.False(t, acquired, "only server expiry can make a live lease available")
	require.Equal(t, idempotency.StatusInProgress, state.Status)
	require.Empty(t, state.LockToken())
	require.NoError(t, store.Complete(ctx, storageKey, []byte("done"), old))
}

func TestNew_RejectsBucketWithoutPerKeyTTL(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	const bucket = "without-key-ttl"
	_, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: bucket, TTL: time.Hour})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour))
	require.ErrorIs(t, err, idempnats.ErrBucketPerKeyTTLRequired)
}

func TestStorage_Complete_ZeroRevisionCannotWrite(t *testing.T) {
	t.Parallel()
	store := setupStorage(t)
	ctx := t.Context()
	require.ErrorIs(t, store.Complete(ctx, "absent", []byte("forged"), make([]byte, 8)), storages.ErrLockStolen)
}

func TestNew_RejectsBucketWithMultipleRevisions(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	const bucket = "with-history"
	_, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket: bucket, TTL: time.Hour, LimitMarkerTTL: time.Hour, History: 2,
	})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour))
	require.ErrorIs(t, err, idempnats.ErrBucketHistoryMismatch)
}

func TestStorage_RoundsLeaseUpToServerResolution(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	const bucket = "rounded-ttl"
	ctx := t.Context()
	store, err := idempnats.New(js, idempnats.WithBucket(bucket))
	require.NoError(t, err)

	ok, _, _, err := store.AttemptLockWithTTL(ctx, "key", []byte("lock"), time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	entry, err := store.KV().Get(ctx, "key")
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "KV_"+bucket)
	require.NoError(t, err)
	msg, err := stream.GetMsg(ctx, entry.Revision())
	require.NoError(t, err)
	require.Equal(t, time.Second.String(), msg.Header.Get(jetstream.MsgTTLHeader))
}
