// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

// TestBuild_NatsMigrateBucketTTL pins that storage.nats.migrateBucketTTL
// reaches the NATS storage: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to the configured TTL.
func TestBuild_NatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-idempotency"
	_, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)

	cfg := func(migrate bool) *idempotencyconfig.Config {
		return &idempotencyconfig.Config{
			TTL: 2 * time.Hour,
			Storage: &storageconfig.CacheStorageConfig{
				Type: storageconfig.CacheStorageTypeNats,
				Nats: &storageconfig.NATSConfig{Bucket: bucket, Replicas: 1, MigrateBucketTTL: migrate},
			},
		}
	}

	_, err = factory.New(cfg(false)).UseJetstream(js).Build()
	require.ErrorIs(t, err, idempnats.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	keeper, err := factory.New(cfg(true)).UseJetstream(js).Build()
	require.NoError(t, err)
	require.NotNil(t, keeper)
	require.Equal(t, 2*time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}

// TestBuild_NatsStrictBucketStorage pins that storage.nats.strictBucketStorage
// reaches the NATS storage: a bucket with another storage type is rejected
// instead of being adopted.
func TestBuild_NatsStrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-idempotency-strict"
	testhelpers.CreateNATSKV(t, js, bucket, time.Hour) // memory storage

	cfg := func(strict bool) *idempotencyconfig.Config {
		return &idempotencyconfig.Config{
			TTL: time.Hour,
			Storage: &storageconfig.CacheStorageConfig{
				Type: storageconfig.CacheStorageTypeNats,
				Nats: &storageconfig.NATSConfig{Bucket: bucket, Replicas: 1, StrictBucketStorage: strict},
			},
		}
	}

	_, err := factory.New(cfg(true)).UseJetstream(js).Build()
	require.ErrorIs(t, err, idempnats.ErrBucketStorageMismatch)

	_, err = factory.New(cfg(false)).UseJetstream(js).Build()
	require.NoError(t, err, "without the flag the bucket is adopted")
}
