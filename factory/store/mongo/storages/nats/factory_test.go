// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/store/mongo/storages/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/store/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	mongofactory "github.com/altessa-s/go-atlas/factory/store/mongo"
)

func natsConfig(bucket string, nats storageconfig.NATSConfig) *storageconfig.BackendConfig {
	nats.Bucket = bucket
	return &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: &nats}
}

func newBuilder(cfg *storageconfig.BackendConfig, js jetstream.JetStream) *mongofactory.CursorStorageBuilder {
	return mongofactory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, factory.Builder(js))
}

func TestBuilder_RoundTrip(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	s, err := newBuilder(natsConfig("", storageconfig.NATSConfig{}), js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)

	meta := &mongo.CursorMetadata{CursorId: "c1", Sort: "name"}
	require.NoError(t, s.Store(t.Context(), "key-1", meta))
	got, err := s.Load(t.Context(), "key-1")
	require.NoError(t, err)
	require.Equal(t, "c1", got.CursorId)
	require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, factory.DefaultBucket), "an unnamed bucket falls back to the default")
}

// TestBuilder_Bucket pins how the NATS cursor bucket is created: file
// storage and the builder's TTL.
func TestBuilder_Bucket(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := newBuilder(natsConfig("hourly", storageconfig.NATSConfig{}), js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, "hourly"))
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "hourly"))

	_, err = newBuilder(natsConfig("hourly", storageconfig.NATSConfig{}), js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err, "an existing matching bucket is adopted")
}

// TestBuilder_MigrateBucketTTL pins that a shared bucket's TTL is not
// rewritten unconditionally: a mismatch is rejected unless
// storage.nats.migrateBucketTTL is set.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := newBuilder(natsConfig("cursors", storageconfig.NATSConfig{}), js).WithTTL(time.Hour).Build(t.Context())
	require.NoError(t, err)

	_, err = newBuilder(natsConfig("cursors", storageconfig.NATSConfig{}), js).WithTTL(2 * time.Hour).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, "cursors"), "a rejected build changed the bucket's TTL")

	_, err = newBuilder(natsConfig("cursors", storageconfig.NATSConfig{MigrateBucketTTL: true}), js).
		WithTTL(2 * time.Hour).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, natstest.KVBucketTTL(t, js, "cursors"))
}

// TestBuilder_BucketStorageMismatch pins that a bucket with another storage
// type is rejected and left untouched: the server cannot convert it.
func TestBuilder_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	natstest.CreateNATSKV(t, js, "memory-cursors", time.Hour) // memory storage

	_, err := newBuilder(natsConfig("memory-cursors", storageconfig.NATSConfig{}), js).WithTTL(time.Hour).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "memory-cursors"))
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)

	_, err := factory.Builder(nil)(t.Context(), natsConfig("", storageconfig.NATSConfig{}), 0, logger)
	require.ErrorIs(t, err, factory.ErrJetStreamRequired)

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	_, err = factory.Builder(js)(t.Context(), &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS}, 0, logger)
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}
