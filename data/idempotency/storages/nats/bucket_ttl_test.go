// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

// TestNew_BucketTTLMismatch pins that an existing bucket with a different key
// TTL is rejected rather than rewritten under the processes already using it,
// and is updated only when migration is asked for. Migration rewrites the whole
// bucket config, so the limit-marker TTL (pinned to MaxAge) moves with it.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

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

// TestNew_MigrateLegacyBucketEnablesPerKeyTTL pins that migrating a bucket
// created without a limit-marker TTL (before per-key TTL was enabled) turns
// per-key TTL on, so AttemptLockWithTTL works on the migrated bucket.
func TestNew_MigrateLegacyBucketEnablesPerKeyTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	// Learn the storage type New applies, so the legacy bucket differs only in
	// its TTLs: a storage-type change would be rejected by the server.
	_, err := idempnats.New(js, idempnats.WithBucket("storage-probe"), idempnats.WithReplicas(1))
	require.NoError(t, err)

	const bucket = "legacy-idempotency"
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   bucket,
		TTL:      time.Hour,
		Storage:  testhelpers.KVBucketStorage(t, js, "storage-probe"),
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
	require.Equal(t, 2*time.Hour, status.LimitMarkerTTL(), "migration must enable per-key TTL on a legacy bucket")

	locked, _, _, err := storage.AttemptLockWithTTL(ctx, "key", []byte("v"), time.Minute)
	require.NoError(t, err, "a per-key TTL write must succeed on the migrated bucket")
	require.True(t, locked)
}
