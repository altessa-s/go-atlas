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

	"github.com/altessa-s/go-atlas/factory/reliability/retryqueue/storages/mongo"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
	retrymongo "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/mongo"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type payload struct {
	Note string `bson:"note"`
}

func mongoConfig(ensureIndexes bool) retryqueueconfig.Config {
	return retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{
		Type: retryqueueconfig.StorageTypeMongo,
		Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagField: "flagged", FlagReasonField: "flagReason", EnsureIndexes: ensureIndexes,
		},
	}}
}

// unreachable returns a database on a client that cannot reach a server.
func unreachable(t *testing.T) *mongo.Database {
	t.Helper()
	client, err := mongo.Connect(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1").SetServerSelectionTimeout(200 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })
	return client.Database("retryqueue")
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	cfg := mongoConfig(false)

	_, err := factory.Builder[payload](nil)(t.Context(), cfg.Storage)
	require.ErrorIs(t, err, factory.ErrClientRequired)
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	_, err = factory.Builder[payload](&mongo.Database{})(t.Context(), &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)

	// The store rejects a flag field it owns.
	bad := mongoConfig(false)
	bad.Storage.Mongo.FlagField = "state"
	_, err = factory.Builder[payload](unreachable(t))(t.Context(), bad.Storage)
	require.ErrorIs(t, err, retrymongo.ErrInvalidFlagField)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

// Without ensureIndexes the build performs no I/O, so it succeeds against an
// unreachable server and returns the MongoDB store.
func TestBuilder_NoIndexes(t *testing.T) {
	t.Parallel()
	cfg := mongoConfig(false)
	queue, err := retryqueuefactory.New[payload](&cfg).
		UseStorage(retryqueueconfig.StorageTypeMongo, factory.Builder[payload](unreachable(t))).
		Build(t.Context())
	require.NoError(t, err)
	require.IsType(t, &retrymongo.Store[payload]{}, queue)
	_, ok := queue.(retryqueue.Mover)
	require.True(t, ok)
}

// Index creation runs on the build context: a canceled startup fails the
// build instead of waiting on an unreachable server.
func TestBuilder_IndexCreationFailure(t *testing.T) {
	t.Parallel()
	cfg := mongoConfig(true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := retryqueuefactory.New[payload](&cfg).
		UseStorage(retryqueueconfig.StorageTypeMongo, factory.Builder[payload](unreachable(t))).
		Build(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, coreerrs.ErrUnavailable)
}
