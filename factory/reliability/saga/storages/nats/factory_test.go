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

	"github.com/altessa-s/go-atlas/factory/reliability/saga/storages/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
)

type order struct{}

func orderDef() *saga.Definition[order] {
	return saga.NewDefinition[order]("place-order").
		Step("charge", func(context.Context, *order) error { return nil }).
		MustBuild()
}

// build assembles an orchestrator from cfg with the nats store registered on js.
func build(t *testing.T, cfg *sagaconfig.Config, js jetstream.JetStream) (*saga.Orchestrator[order], error) {
	t.Helper()
	return sagafactory.New(cfg, orderDef()).
		UseStorage(sagaconfig.StorageTypeNATS, factory.Builder(js)).
		Build(t.Context())
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	storage := &sagaconfig.StorageConfig{
		Type: sagaconfig.StorageTypeNATS,
		NATS: &sagaconfig.NATSStorageConfig{Bucket: "saga", MaxAge: 720 * time.Hour},
	}
	_, err := factory.Builder(nil)(t.Context(), storage)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	_, err = factory.Builder(js)(t.Context(), &sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeNATS})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

// TestBuilder_MigrateBucketTTL pins that storage.nats.migrate_bucket_ttl
// reaches the NATS store: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to max_age.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	const bucket = "factory-saga"
	_, err := natsstore.New(t.Context(), js, natsstore.WithBucket(bucket), natsstore.WithBucketTTL(time.Hour))
	require.NoError(t, err)

	cfg := func(migrate bool) *sagaconfig.Config {
		c := sagaconfig.Default()
		c.Storage = &sagaconfig.StorageConfig{
			Type: sagaconfig.StorageTypeNATS,
			NATS: &sagaconfig.NATSStorageConfig{Bucket: bucket, MaxAge: 2 * time.Hour, MigrateBucketTTL: migrate},
		}
		return &c
	}

	_, err = build(t, cfg(false), js)
	require.ErrorIs(t, err, natsstore.ErrBucketTTLMismatch)
	require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	_, err = build(t, cfg(true), js)
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, natstest.KVBucketTTL(t, js, bucket), "migrate_bucket_ttl must update the bucket")
}
