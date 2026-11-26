// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	idempnats "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

// TestNew_BucketTTLMismatch pins that an existing bucket with a different key
// TTL is rejected rather than rewritten under the processes already using it,
// and is updated only when migration is asked for. Migration rewrites the whole
// bucket config, so the limit-marker TTL (pinned to MaxAge) moves with it.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	const bucket = "shared-idempotency"

	_, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour))
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour))
	require.ErrorIs(t, err, idempnats.ErrBucketTTLMismatch)

	kv, err := js.KeyValue(t.Context(), bucket)
	require.NoError(t, err)
	status, err := kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.TTL(), "a rejected New changed the shared bucket's TTL")

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour), idempnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	status, err = kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, status.TTL(), "WithMigrateBucketTTL must update the bucket")
	require.Equal(t, 2*time.Hour, status.LimitMarkerTTL(), "migration also moves the marker TTL, which follows MaxAge")
}

func TestStorage_LeaseUsesCallerTTLAfterBucketMigration(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, acquiringJS := natstest.ConnectJetStream(t, ns)
	_, migratingJS := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()

	const bucket = "lease-ttl-migration"
	const lease = 5 * time.Minute
	storage, err := idempnats.New(acquiringJS, idempnats.WithBucket(bucket), idempnats.WithMaxAge(lease))
	require.NoError(t, err)

	_, err = idempnats.New(migratingJS, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour), idempnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	const key = "key"
	locked, _, _, err := storage.AttemptLockWithTTL(ctx, key, []byte("lock"), lease)
	require.NoError(t, err)
	require.True(t, locked)

	stream, err := acquiringJS.Stream(ctx, "KV_"+bucket)
	require.NoError(t, err)
	require.Equal(t, time.Hour, stream.CachedInfo().Config.MaxAge)
	msg, err := stream.GetLastMsgForSubject(ctx, "$KV."+bucket+"."+key)
	require.NoError(t, err)
	require.Equal(t, lease.String(), msg.Header.Get(jetstream.MsgTTLHeader))
}

func TestNew_MigrateBucketWithMatchingTTLEnablesPerKeyTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()

	const bucket = "matching-ttl-idempotency"
	const description = "existing idempotency records"
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucket, TTL: time.Hour, Description: description,
	})
	require.NoError(t, err)
	const existingKey = "existing"
	value := []byte("existing record")
	revision, err := kv.Put(ctx, existingKey, value)
	require.NoError(t, err)

	storage, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour), idempnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	stream, err := js.Stream(ctx, "KV_"+bucket)
	require.NoError(t, err)
	cfg := stream.CachedInfo().Config
	require.True(t, cfg.AllowMsgTTL)
	require.Equal(t, time.Hour, cfg.MaxAge)
	require.Equal(t, time.Hour, cfg.SubjectDeleteMarkerTTL)
	require.Equal(t, description, cfg.Description)
	entry, err := kv.Get(ctx, existingKey)
	require.NoError(t, err)
	require.Equal(t, revision, entry.Revision())
	require.Equal(t, value, entry.Value())

	locked, _, _, err := storage.AttemptLockWithTTL(ctx, "key", []byte("lock"), time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
}

// TestNew_MigrateBucketWithoutMarkerTTLEnablesPerKeyTTL pins that migrating a
// bucket created without a limit-marker TTL turns per-key TTL on, so
// AttemptLockWithTTL works on the migrated bucket.
func TestNew_MigrateBucketWithoutMarkerTTLEnablesPerKeyTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()

	// Learn the storage type New applies, so the existing bucket differs only in
	// its TTLs: a storage-type change would be rejected by the server.
	_, err := idempnats.New(js, idempnats.WithBucket("storage-probe"), idempnats.WithReplicas(1))
	require.NoError(t, err)

	const bucket = "no-marker-idempotency"
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   bucket,
		TTL:      time.Hour,
		Storage:  natstest.KVBucketStorage(t, js, "storage-probe"),
		Replicas: 1,
	})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour), idempnats.WithReplicas(1))
	require.ErrorIs(t, err, idempnats.ErrBucketTTLMismatch)

	status, err := kv.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.TTL(), "a rejected New changed the bucket's TTL")
	require.Zero(t, status.LimitMarkerTTL(), "a rejected New changed the bucket's marker TTL")

	storage, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour), idempnats.WithReplicas(1),
		idempnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	status, err = kv.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, status.TTL())
	require.Equal(t, 2*time.Hour, status.LimitMarkerTTL(), "migration must enable per-key TTL on a bucket without one")

	locked, _, _, err := storage.AttemptLockWithTTL(ctx, "key", []byte("v"), time.Minute)
	require.NoError(t, err, "a per-key TTL write must succeed on the migrated bucket")
	require.True(t, locked)
}
