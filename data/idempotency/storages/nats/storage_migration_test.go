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

// TestMigrateBucketStorage moves a memory bucket, as earlier releases created
// it, to file storage: a completed key stays completed, an in-progress lock
// keeps its per-key TTL and stays held, and revisions keep growing.
func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()
	opts := []idempnats.Option{idempnats.WithBucket("idem"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1)}

	_, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "idem", TTL: time.Hour, Storage: jetstream.MemoryStorage, LimitMarkerTTL: time.Hour,
	})
	require.NoError(t, err)
	storage, err := idempnats.New(js, opts...)
	require.NoError(t, err)

	ok, _, token, err := storage.AttemptLock(ctx, "done", []byte("pending"))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, storage.Complete(ctx, "done", []byte("result"), token))
	ok, _, _, err = storage.AttemptLockWithTTL(ctx, "busy", []byte("pending"), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	before := testhelpers.KVBucketStorage(t, js, "idem")
	require.Equal(t, jetstream.MemoryStorage, before)

	require.NoError(t, idempnats.MigrateBucketStorage(ctx, js, idempnats.MigrationOptions{}, opts...))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "idem"))

	migrated, err := idempnats.New(js, append(opts, idempnats.WithStrictBucketStorage())...)
	require.NoError(t, err)
	ok, existing, _, err := migrated.AttemptLock(ctx, "done", []byte("again"))
	require.NoError(t, err)
	require.False(t, ok, "a completed key must still deduplicate")
	require.Equal(t, "result", string(existing))
	ok, existing, _, err = migrated.AttemptLock(ctx, "busy", []byte("again"))
	require.NoError(t, err)
	require.False(t, ok, "an in-progress lock must still be held")
	require.Equal(t, "pending", string(existing))

	stream, err := js.Stream(ctx, "KV_idem")
	require.NoError(t, err)
	busy, err := stream.GetLastMsgForSubject(ctx, "$KV.idem.busy")
	require.NoError(t, err)
	require.NotEmpty(t, busy.Header.Get("Nats-TTL"), "the lock must keep its per-key TTL")
}

func TestMigrateBucketStorage_InProgress(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	// A marker of an unfinished migration.
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "KVMIGRATE_idem", Subjects: []string{"_kvmigrate.idem.>"}})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket("idem"))
	require.ErrorIs(t, err, idempnats.ErrBucketMigrationInProgress)
	err = idempnats.MigrateBucketStorage(t.Context(), js, idempnats.MigrationOptions{}, idempnats.WithBucket("idem"))
	require.ErrorIs(t, err, idempnats.ErrBucketMigrationInProgress)
}

// TestMigrateBucketStorage_Mirror migrates a mirror of an idempotency bucket
// through the public wrapper with its normal options, which ask for per-key
// TTL — something a mirror cannot have — and checks it is recreated as the
// same mirror on file storage.
func TestMigrateBucketStorage_Mirror(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	origin, err := idempnats.New(js, idempnats.WithBucket("origin"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)
	ok, _, token, err := origin.AttemptLock(ctx, "done", []byte("pending"))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, origin.Complete(ctx, "done", []byte("result"), token))

	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "replica", TTL: time.Hour, Storage: jetstream.MemoryStorage, Mirror: &jetstream.StreamSource{Name: "origin"},
	})
	require.NoError(t, err)

	err = idempnats.MigrateBucketStorage(ctx, js, idempnats.MigrationOptions{},
		idempnats.WithBucket("replica"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)

	stream, err := js.Stream(ctx, "KV_replica")
	require.NoError(t, err)
	cfg := stream.CachedInfo().Config
	require.Equal(t, jetstream.FileStorage, cfg.Storage)
	require.NotNil(t, cfg.Mirror)
	require.Equal(t, "KV_origin", cfg.Mirror.Name)
	require.Eventually(t, func() bool {
		msg, err := stream.GetLastMsgForSubject(ctx, "$KV.origin.done")
		return err == nil && string(msg.Data) == "result"
	}, 5*time.Second, 50*time.Millisecond, "the new mirror must re-sync from the origin")
}
