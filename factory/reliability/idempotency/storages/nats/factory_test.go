// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	idempotencyfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	idempnats "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

// build assembles a Keeper from cfg with the nats storage registered on js.
func build(cfg *idempotencyconfig.Config, js jetstream.JetStream) (*idempotency.Keeper, error) {
	return idempotencyfactory.New(cfg).UseStorage(idempotencyconfig.StorageTypeNATS, factory.Builder(js)).Build()
}

// TestBuilder_MigrateBucketTTL pins that storage.nats.migrateBucketTTL
// reaches the NATS storage: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to the configured TTL.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	const bucket = "factory-idempotency"
	_, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)

	cfg := func(migrate bool) *idempotencyconfig.Config {
		return &idempotencyconfig.Config{
			TTL: 2 * time.Hour,
			Storage: &idempotencyconfig.StorageConfig{
				Type: idempotencyconfig.StorageTypeNATS,
				NATS: &storageconfig.NATSConfig{Bucket: bucket, Replicas: 1, MigrateBucketTTL: migrate},
			},
		}
	}

	_, err = build(cfg(false), js)
	require.ErrorIs(t, err, idempnats.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	keeper, err := build(cfg(true), js)
	require.NoError(t, err)
	require.NotNil(t, keeper)
	require.Equal(t, 2*time.Hour, natstest.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}
func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	cfg := &idempotencyconfig.Config{TTL: time.Hour, Storage: &idempotencyconfig.StorageConfig{
		Type: idempotencyconfig.StorageTypeNATS, NATS: &storageconfig.NATSConfig{},
	}}
	_, err := factory.Builder(nil)(cfg)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	_, err = build(&idempotencyconfig.Config{TTL: time.Hour, Storage: &idempotencyconfig.StorageConfig{
		Type: idempotencyconfig.StorageTypeNATS,
	}}, js)
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}
