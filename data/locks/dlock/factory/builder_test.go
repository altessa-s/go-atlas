// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/locks/dlock/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// TestBuild_NatsMigrateBucketTTL pins that nats.migrateBucketTTL reaches the
// NATS provider: without it a bucket with a different key TTL is rejected and
// left alone, with it the bucket is migrated to the provider's lock TTL.
func TestBuild_NatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	const bucket = "factory-dlock"
	first, err := locknats.New(t.Context(), nc, locknats.WithBucket(bucket), locknats.WithTTL(time.Minute))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close(context.Background()) })

	cfg := func(migrate bool) *config.DistributionLock {
		return &config.DistributionLock{
			Provider: config.DistributionLockProviderNats,
			Nats:     &config.DistributionLockNats{Bucket: bucket, MigrateBucketTTL: migrate},
		}
	}

	_, err = factory.New(cfg(false)).UseNatsConn(nc).Build(t.Context())
	require.ErrorIs(t, err, locknats.ErrBucketTTLMismatch)
	require.Equal(t, time.Minute, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	dl, err := factory.New(cfg(true)).UseNatsConn(nc).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, dl)
	require.Equal(t, locknats.DefaultBucketKeysTTL, testhelpers.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}

func TestBuild_NilNatsSection(t *testing.T) {
	t.Parallel()

	_, err := factory.New(&config.DistributionLock{Provider: config.DistributionLockProviderNats}).Build(t.Context())
	require.Error(t, err)
}
