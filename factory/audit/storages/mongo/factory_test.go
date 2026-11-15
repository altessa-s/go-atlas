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

	"github.com/altessa-s/go-atlas/factory/audit/storages/mongo"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	auditfactory "github.com/altessa-s/go-atlas/factory/audit"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func mongoConfig() *auditconfig.Config {
	return &auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type:  auditconfig.StorageTypeMongo,
			Mongo: &auditconfig.StorageMongo{CollectionName: "audit_events", IndexTimeout: time.Second},
		},
	}
}

func buildStorage(ctx context.Context, cfg *auditconfig.Config, db *mongo.Database) error {
	_, err := auditfactory.New(cfg).
		UseStorage(auditconfig.StorageTypeMongo, factory.Builder(db)).
		BuildStorage(ctx)
	return err
}

// Mongo storage needs a database; without one the failure must be explicit
// rather than a silent fallback to memory.
func TestBuilder_RequiresDatabase(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, buildStorage(t.Context(), mongoConfig(), nil), factory.ErrDatabaseRequired)
}

// Index creation runs on the build context, so a canceled startup fails the
// build instead of hanging on an unreachable server.
func TestBuilder_IndexCreationRunsOnBuildContext(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect(mongooptions.Client().
		ApplyURI("mongodb://127.0.0.1:1/?directConnection=true").
		SetServerSelectionTimeout(200 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cfg := mongoConfig()
	cfg.Storage.Mongo = nil // the section is optional: all defaults
	err = buildStorage(ctx, cfg, client.Database("audit"))
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, factory.ErrDatabaseRequired)
}
