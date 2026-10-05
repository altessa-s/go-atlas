// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

// TestBuildNatsMigrateBucketTTL pins that storage.nats.migrate_bucket_ttl
// reaches the NATS store: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to max_age.
func TestBuildNatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-saga"
	_, err := natsstore.New(js, natsstore.WithBucket(bucket), natsstore.WithBucketTTL(time.Hour))
	require.NoError(t, err)

	cfg := func(migrate bool) *config.Saga {
		c := config.DefaultSaga()
		c.Storage = &config.SagaStorageConfig{
			Type: config.SagaStorageTypeNats,
			Nats: &config.SagaNatsStorageConfig{Bucket: bucket, MaxAge: 2 * time.Hour, MigrateBucketTTL: migrate},
		}
		return &c
	}

	_, err = sagafactory.New(cfg(false), orderDef()).UseJetStream(js).Build()
	require.ErrorIs(t, err, natsstore.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	_, err = sagafactory.New(cfg(true), orderDef()).UseJetStream(js).Build()
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "migrate_bucket_ttl must update the bucket")
}

// TestBuildNatsStrictBucketStorage pins that storage.nats.strict_bucket_storage
// reaches the NATS store: a bucket with another storage type is rejected
// instead of being adopted.
func TestBuildNatsStrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-saga-strict"
	testhelpers.CreateNATSKV(t, js, bucket, time.Hour) // memory storage

	cfg := func(strict bool) *config.Saga {
		c := config.DefaultSaga()
		c.Storage = &config.SagaStorageConfig{
			Type: config.SagaStorageTypeNats,
			Nats: &config.SagaNatsStorageConfig{Bucket: bucket, MaxAge: time.Hour, StrictBucketStorage: strict},
		}
		return &c
	}

	_, err := sagafactory.New(cfg(true), orderDef()).UseJetStream(js).Build()
	require.ErrorIs(t, err, natsstore.ErrBucketStorageMismatch)

	_, err = sagafactory.New(cfg(false), orderDef()).UseJetStream(js).Build()
	require.NoError(t, err, "without the flag the bucket is adopted")
}
