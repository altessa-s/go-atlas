// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/limiters/budget/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

// TestBuild_NatsMigrateBucketTTL pins that storage.nats.migrateBucketTTL
// reaches the NATS storage: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to the storage TTL.
func TestBuild_NatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-budget"
	_, err := limitnats.New(js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(time.Hour))
	require.NoError(t, err)

	cfg := func(migrate bool) *config.BudgetLimiter {
		storage := &config.CacheStorageConfig{
			Type: config.CacheStorageTypeNats,
			Nats: &config.StorageNATSConfig{Bucket: bucket, Replicas: 1, MigrateBucketTTL: migrate},
		}
		return &config.BudgetLimiter{
			Limit:   1000,
			Period:  time.Hour,
			Storage: storage,
		}
	}

	_, err = factory.New(cfg(false)).UseJetstream(js).Build()
	require.ErrorIs(t, err, limitnats.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	_, err = factory.New(cfg(true)).UseJetstream(js).Build()
	require.NoError(t, err)
	require.Equal(t, limitnats.DefaultMaxAge, testhelpers.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}
