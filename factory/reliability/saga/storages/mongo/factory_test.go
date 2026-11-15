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

	"github.com/altessa-s/go-atlas/factory/reliability/saga/storages/mongo"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type order struct{}

func orderDef() *saga.Definition[order] {
	return saga.NewDefinition[order]("place-order").
		Step("charge", func(context.Context, *order) error { return nil }).
		MustBuild()
}

func mongoStorage() *sagaconfig.StorageConfig {
	return &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeMongo,
		Mongo: &sagaconfig.MongoStorageConfig{Collection: "saga_instances"},
	}
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(), mongoStorage())
	require.ErrorIs(t, err, factory.ErrClientRequired)

	_, err = factory.Builder(&mongo.Database{})(t.Context(), &sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeMongo})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}

// Mongo index creation runs on the build context: a canceled startup fails
// the build instead of waiting on an unreachable server.
func TestBuilder_IndexCreationFailure(t *testing.T) {
	t.Parallel()
	client, err := mongo.Connect(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1").SetServerSelectionTimeout(200 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

	cfg := sagaconfig.Default()
	cfg.Storage = mongoStorage()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = sagafactory.New(&cfg, orderDef()).
		UseStorage(sagaconfig.StorageTypeMongo, factory.Builder(client.Database("saga"))).
		Build(ctx)
	require.Error(t, err)
}
