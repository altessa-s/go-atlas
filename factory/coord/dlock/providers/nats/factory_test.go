// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/coord/dlock/providers/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	locknats "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// newBuilder returns a dlock builder with the nats provider registered on nc.
func newBuilder(cfg *lockconfig.DistributionLock, nc *nats.Conn) *dlockfactory.DLockBuilder {
	return dlockfactory.New(cfg).UseProvider(lockconfig.DistributionLockProviderNATS, factory.Builder(nc))
}

// TestBuilder_MigrateBucketTTL pins that nats.migrateBucketTTL reaches the
// NATS provider: without it a bucket with a different key TTL is rejected and
// left alone, with it the bucket is migrated to the provider's lock TTL.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	const bucket = "factory-dlock"
	first, err := locknats.New(t.Context(), nc, locknats.WithBucket(bucket), locknats.WithTTL(time.Minute))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close(context.WithoutCancel(t.Context())) })

	cfg := func(migrate bool) *lockconfig.DistributionLock {
		return &lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: bucket, MigrateBucketTTL: migrate},
		}
	}

	_, err = newBuilder(cfg(false), nc).Build(t.Context())
	require.ErrorIs(t, err, locknats.ErrBucketTTLMismatch)
	require.Equal(t, time.Minute, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	dl, err := newBuilder(cfg(true), nc).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, dl)
	require.Equal(t, locknats.DefaultBucketKeysTTL, natstest.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}

// TestBuilder_Storage pins that nats.storage selects the storage type of the
// bucket the provider creates, memory by default.
func TestBuilder_Storage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)

	for bucket, storage := range map[string]storageconfig.KVStorageType{"locks-default": "", "locks-memory": storageconfig.KVStorageMemory, "locks-file": storageconfig.KVStorageFile} {
		dl, err := newBuilder(&lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: bucket, Storage: storage},
		}, nc).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, dl)

		want := jetstream.MemoryStorage
		if storage == storageconfig.KVStorageFile {
			want = jetstream.FileStorage
		}
		require.Equal(t, want, natstest.KVBucketStorage(t, js, bucket), bucket)
	}
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)

	_, err := factory.Builder(nil)(t.Context(), &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNATS}, logger)
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)

	_, err = newBuilder(&lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderNATS,
		NATS:     &lockconfig.DistributionLockNATS{Bucket: "locks"},
	}, nil).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrClientRequired)
}
