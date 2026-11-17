// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/factory/reliability/outbox/storages/mongo"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func noopHandler(context.Context, outbox.Event) error { return nil }

// offlineMongo returns a lazily connected client; nothing is dialed until an
// operation runs.
func offlineMongo(t *testing.T) *mongo.Client {
	t.Helper()
	client, err := mongo.Connect(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1").SetServerSelectionTimeout(200 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })
	return client
}

// build builds an outbox over the MongoDB storage function.
func build(ctx context.Context, fn outboxfactory.StorageFunc) error {
	cfg := &brokerconfig.Outbox{Enabled: true}
	_, err := outboxfactory.New(cfg).UseStorage(brokerconfig.OutboxStorageTypeMongo, fn).Build(ctx, noopHandler)
	return err
}

func TestBuilder_RequiresDependency(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, build(t.Context(), factory.Builder(nil)), factory.ErrDatabaseRequired)
	require.ErrorIs(t, build(t.Context(), factory.CollectionBuilder(nil)), factory.ErrCollectionRequired)
}

// Index creation runs on the build context, so a canceled startup fails the
// build instead of hanging on an unreachable server.
func TestBuilder_IndexCreationRunsOnBuildContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	db := offlineMongo(t).Database("outbox")

	require.ErrorIs(t, build(ctx, factory.Builder(db)), context.Canceled)
	require.ErrorIs(t, build(ctx, factory.CollectionBuilder(db.Collection("events"))), context.Canceled)
}
