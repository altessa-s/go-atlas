// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

func natsConfig(bucket string, nats storageconfig.NATSConfig) *storageconfig.CacheStorageConfig {
	nats.Bucket = bucket
	return &storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeNATS, NATS: &nats}
}

// TestBuild_NatsBucket pins how the NATS cursor bucket is created: file
// storage, the builder's TTL, and no TTL at all — cursors never expire — when
// WithTTL is not called, as before the factory went through natskvlease.
func TestBuild_NatsBucket(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	_, err := factory.New(natsConfig("forever", storageconfig.NATSConfig{})).UseJetstream(js).Build(t.Context())
	require.NoError(t, err)
	require.Zero(t, testhelpers.KVBucketTTL(t, js, "forever"))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "forever"))

	_, err = factory.New(natsConfig("hourly", storageconfig.NATSConfig{})).UseJetstream(js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, "hourly"))

	_, err = factory.New(natsConfig("hourly", storageconfig.NATSConfig{})).UseJetstream(js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err, "an existing matching bucket is adopted")
}

// TestBuild_NatsMigrateBucketTTL pins that the factory no longer rewrites a
// shared bucket's TTL unconditionally: a mismatch is rejected unless
// storage.nats.migrateBucketTTL is set.
func TestBuild_NatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	_, err := factory.New(natsConfig("cursors", storageconfig.NATSConfig{})).UseJetstream(js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)

	_, err = factory.New(natsConfig("cursors", storageconfig.NATSConfig{})).UseJetstream(js).WithTTL(2 * time.Hour).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, "cursors"), "a rejected build changed the bucket's TTL")

	_, err = factory.New(natsConfig("cursors", storageconfig.NATSConfig{MigrateBucketTTL: true})).
		UseJetstream(js).WithTTL(2 * time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, testhelpers.KVBucketTTL(t, js, "cursors"))
}

// TestBuild_NatsStrictBucketStorage pins that storage.nats.strictBucketStorage
// rejects a bucket with another storage type, which is otherwise adopted.
func TestBuild_NatsStrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	testhelpers.CreateNATSKV(t, js, "memory-cursors", time.Hour) // memory storage

	_, err := factory.New(natsConfig("memory-cursors", storageconfig.NATSConfig{StrictBucketStorage: true})).
		UseJetstream(js).WithTTL(time.Hour).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrBucketStorageMismatch)

	_, err = factory.New(natsConfig("memory-cursors", storageconfig.NATSConfig{})).UseJetstream(js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "memory-cursors"))
}
