// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/leadelect/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// TestBuild_MigrateBucketTTL pins that migrateBucketTTL reaches the NATS
// provider: without it an election bucket with a different key TTL is rejected
// and left alone, with it the bucket is migrated to the provider's key TTL.
func TestBuild_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// The provider's default bucket and storage; only the key TTL differs.
	const bucket = lenats.DefaultBucket
	_, err = js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:  bucket,
		TTL:     time.Minute,
		Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	cfg := func(migrate bool) *config.LeaderElector {
		return &config.LeaderElector{
			Provider:         config.LeaderElectorProviderNats,
			Ttl:              10 * time.Second,
			MigrateBucketTTL: migrate,
		}
	}

	_, err = factory.New(cfg(false)).UseNatsConn(nc).Build(t.Context())
	require.ErrorIs(t, err, lenats.ErrBucketTTLMismatch)
	require.Equal(t, time.Minute, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	leader, err := factory.New(cfg(true)).UseNatsConn(nc).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, leader)
	require.Equal(t, lenats.DefaultBucketKeysTTL, testhelpers.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}
