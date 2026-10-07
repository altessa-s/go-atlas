// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
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

	cfg := func(migrate bool) *lockconfig.LeaderElector {
		return &lockconfig.LeaderElector{
			Provider:         lockconfig.LeaderElectorProviderNats,
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

// TestBuild_StrictBucketStorage pins that strictBucketStorage reaches the NATS
// provider: an election bucket with another storage type is rejected instead
// of being adopted.
func TestBuild_StrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// The provider asks for memory storage; this bucket is file-backed.
	_, err = js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:  lenats.DefaultBucket,
		TTL:     lenats.DefaultBucketKeysTTL,
		Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)

	cfg := func(strict bool) *lockconfig.LeaderElector {
		return &lockconfig.LeaderElector{
			Provider:            lockconfig.LeaderElectorProviderNats,
			Ttl:                 10 * time.Second,
			StrictBucketStorage: strict,
		}
	}

	_, err = factory.New(cfg(true)).UseNatsConn(nc).Build(t.Context())
	require.ErrorIs(t, err, lenats.ErrBucketStorageMismatch)

	_, err = factory.New(cfg(false)).UseNatsConn(nc).Build(t.Context())
	require.NoError(t, err, "without the flag the bucket is adopted")
}

// TestBuild_Storage pins that storage selects the storage type of the election
// bucket the provider creates, memory by default.
func TestBuild_Storage(t *testing.T) {
	t.Parallel()

	for name, storage := range map[string]storageconfig.KVStorageType{"default": "", "file": storageconfig.KVStorageFile} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ns := testhelpers.StartNATSServer(t)
			nc, js := testhelpers.ConnectJetStream(t, ns)

			_, err := factory.New(&lockconfig.LeaderElector{
				Provider: lockconfig.LeaderElectorProviderNats,
				Ttl:      10 * time.Second,
				Storage:  storage,
			}).UseNatsConn(nc).Build(t.Context())
			require.NoError(t, err)

			want := jetstream.MemoryStorage
			if storage == storageconfig.KVStorageFile {
				want = jetstream.FileStorage
			}
			require.Equal(t, want, testhelpers.KVBucketStorage(t, js, lenats.DefaultBucket))
		})
	}
}
