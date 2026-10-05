// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/idempotency/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

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

	cfg := func(migrate bool) *config.Idempotency {
		return &config.Idempotency{
			TTL: 2 * time.Hour,
			Storage: &config.CacheStorageConfig{
				Type: config.CacheStorageTypeNats,
				Nats: &config.StorageNATSConfig{Bucket: bucket, Replicas: 1, MigrateBucketTTL: migrate},
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
