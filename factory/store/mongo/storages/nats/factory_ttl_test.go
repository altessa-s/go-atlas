// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/store/mongo/storages/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/store/mongo"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

// TestBuilder_RequiresPositiveTTL pins that the NATS cursor storage is never
// built without a key TTL — nothing else removes cursors — and that a rejected
// build creates no bucket.
func TestBuilder_RequiresPositiveTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	tests := []struct {
		name   string
		bucket string
		ttl    time.Duration
	}{
		{name: "zero", bucket: "zero-ttl", ttl: 0},
		{name: "negative", bucket: "negative-ttl", ttl: -time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newBuilder(natsConfig(tc.bucket, storageconfig.NATSConfig{}), js).WithTTL(tc.ttl).Build(t.Context())
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

			_, err = js.KeyValue(t.Context(), tc.bucket)
			require.ErrorIs(t, err, jetstream.ErrBucketNotFound)
		})
	}

	t.Run("unset", func(t *testing.T) {
		t.Parallel()

		_, err := newBuilder(natsConfig("unset-ttl", storageconfig.NATSConfig{}), js).Build(t.Context())
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	})
}

// TestBuilder_ExistingBucketWithoutTTL pins how a bucket created without a
// key TTL is handled: rejected and left untouched by default, given the
// builder's TTL in place with storage.nats.migrateBucketTTL, after which the
// cursors already in it expire.
func TestBuilder_ExistingBucketWithoutTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	const bucket = "legacy-cursors"
	kv, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: bucket, Storage: jetstream.FileStorage})
	require.NoError(t, err)
	_, err = kv.Put(t.Context(), "stale", []byte(`{"cursorId":"old"}`))
	require.NoError(t, err)

	_, err = newBuilder(natsConfig(bucket, storageconfig.NATSConfig{}), js).WithTTL(time.Second).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrBucketTTLMismatch)
	require.Zero(t, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	s, err := newBuilder(natsConfig(bucket, storageconfig.NATSConfig{MigrateBucketTTL: true}), js).
		WithTTL(time.Second).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Second, natstest.KVBucketTTL(t, js, bucket))

	require.Eventually(t, func() bool {
		_, err := s.Load(t.Context(), "stale")
		return errors.Is(err, mongo.ErrCursorNotFound)
	}, 15*time.Second, 100*time.Millisecond, "a cursor stored before the migration outlived the new key TTL")
}
