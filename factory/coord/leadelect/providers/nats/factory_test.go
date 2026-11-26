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

	"github.com/altessa-s/go-atlas/factory/coord/leadelect/providers/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
	leadelectfactory "github.com/altessa-s/go-atlas/factory/coord/leadelect"
)

// newBuilder returns a leadelect builder with the nats provider registered on nc.
func newBuilder(cfg *lockconfig.LeaderElector, nc *nats.Conn) *leadelectfactory.LeaderBuilder {
	return leadelectfactory.New(cfg).UseProvider(lockconfig.LeaderElectorProviderNATS, factory.Builder(nc))
}

// TestBuilder_MigrateBucketTTL pins that migrateBucketTTL reaches the NATS
// provider: without it an election bucket with a different key TTL is rejected
// and left alone, with it the bucket is migrated to the provider's key TTL.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
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
			Provider:         lockconfig.LeaderElectorProviderNATS,
			Ttl:              10 * time.Second,
			MigrateBucketTTL: migrate,
		}
	}

	_, err = newBuilder(cfg(false), nc).WithNodeId("node").Build(t.Context())
	require.ErrorIs(t, err, lenats.ErrBucketTTLMismatch)
	require.Equal(t, time.Minute, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	leader, err := newBuilder(cfg(true), nc).WithNodeId("node").Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, leader)
	require.Equal(t, lenats.DefaultBucketKeysTTL, natstest.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}

// TestBuilder_Storage pins that storage selects the storage type of the election
// bucket the provider creates, memory by default.
func TestBuilder_Storage(t *testing.T) {
	t.Parallel()

	for name, storage := range map[string]storageconfig.KVStorageType{"default": "", "file": storageconfig.KVStorageFile} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ns := natstest.StartNATSServer(t)
			nc, js := natstest.ConnectJetStream(t, ns)

			_, err := newBuilder(&lockconfig.LeaderElector{
				Provider: lockconfig.LeaderElectorProviderNATS,
				Ttl:      10 * time.Second,
				Storage:  storage,
			}, nc).WithNodeId("node").Build(t.Context())
			require.NoError(t, err)

			want := jetstream.MemoryStorage
			if storage == storageconfig.KVStorageFile {
				want = jetstream.FileStorage
			}
			require.Equal(t, want, natstest.KVBucketStorage(t, js, lenats.DefaultBucket))
		})
	}
}

func TestBuilder_ElectsConfiguredNode(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)

	leader, err := newBuilder(&lockconfig.LeaderElector{Ttl: 5 * time.Second}, nc).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseDefaultLogger().
		UseMeter(metricstest.NewTestCollector()).
		WithKey("  election-a  ").
		WithKey("   "). // blank values keep the previous setting
		WithNodeId(" node-1 ").
		WithNodeId("").
		Build(t.Context())
	require.NoError(t, err)

	require.NoError(t, leader.Start(t.Context()))
	t.Cleanup(func() { _ = leader.Stop(context.WithoutCancel(t.Context())) })
	require.Equal(t, "node-1", leader.NodeId())

	testhelpers.WaitFor(t, 10*time.Second, leader.IsLeader, "the only node must become leader")
	id, err := leader.LeaderId(t.Context())
	require.NoError(t, err)
	require.Equal(t, "node-1", id)
}

func TestBuilder_RequiresConnection(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(), &lockconfig.LeaderElector{Ttl: time.Second}, slog.New(slog.DiscardHandler), nil)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	_, err = newBuilder(&lockconfig.LeaderElector{Ttl: time.Second}, nil).WithNodeId("node").Build(t.Context())
	require.ErrorIs(t, err, factory.ErrClientRequired)
}
