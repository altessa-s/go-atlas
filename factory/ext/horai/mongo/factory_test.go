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

	"github.com/altessa-s/go-atlas/factory/ext/horai/mongo"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func mongoConfig(ensureIndexes bool) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	storageCfg := schedulerconfig.DefaultStorageMongoConfig()
	storageCfg.EnsureIndexes = ensureIndexes
	cfg.Storage = &schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeMongo, Mongo: &storageCfg}
	return &cfg
}

// build builds a scheduler with the MongoDB task storage registered over db.
func build(t *testing.T, cfg *schedulerconfig.Config, db *mongo.Database) error {
	t.Helper()
	s, err := schedulerfactory.New(cfg).
		UseStorage(schedulerconfig.StorageTypeMongo, factory.Builder(db)).
		Build(t.Context())
	if err == nil {
		require.NotNil(t, s)
	}
	return err
}

func TestBuilder(t *testing.T) {
	t.Parallel()
	// Connect performs no I/O; server selection against the closed port fails
	// fast once an operation runs.
	client, err := mongo.Connect(mongooptions.Client().
		ApplyURI("mongodb://127.0.0.1:1/?directConnection=true").
		SetServerSelectionTimeout(100 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })
	db := client.Database("scheduler_factory_test")

	t.Run("requires_database", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, build(t, mongoConfig(false), nil), factory.ErrDatabaseRequired)
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		cfg := mongoConfig(false)
		cfg.Storage.Mongo = nil
		require.ErrorIs(t, build(t, cfg, db), factory.ErrConfigRequired)
	})

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, build(t, mongoConfig(false), db))
	})

	t.Run("ensure_indexes_failure", func(t *testing.T) {
		t.Parallel()
		err := build(t, mongoConfig(true), db)
		require.True(t, mongo.IsTimeout(err), "want server selection timeout, got %v", err)
	})
}
