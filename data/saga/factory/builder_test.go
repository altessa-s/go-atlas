// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
)

type order struct {
	Charged bool
}

func orderDef() *saga.Definition[order] {
	return saga.NewDefinition[order]("place-order").
		Step("charge", func(_ context.Context, o *order) error {
			o.Charged = true
			return nil
		}).
		MustBuild()
}

func TestBuildNilConfig(t *testing.T) {
	t.Parallel()
	_, err := sagafactory.New(nil, orderDef()).Build()
	require.ErrorContains(t, err, "configuration is required")
}

func TestBuildNilDefinition(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()
	_, err := sagafactory.New[order](&cfg, nil).Build()
	require.ErrorContains(t, err, "definition is required")
}

func TestBuildMemory(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()

	orch, err := sagafactory.New(&cfg, orderDef()).Build()
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}

func TestBuildNatsRequiresJetStream(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()
	cfg.Storage = &config.SagaStorageConfig{
		Type: config.SagaStorageTypeNats,
		Nats: &config.SagaNatsStorageConfig{Bucket: "saga", MaxAge: 720 * time.Hour},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "NATS JetStream is required")
}

func TestBuildMongoRequiresDatabase(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()
	cfg.Storage = &config.SagaStorageConfig{
		Type:  config.SagaStorageTypeMongo,
		Mongo: &config.SagaMongoStorageConfig{Collection: "saga_instances"},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "MongoDB database is required")
}

func TestBuildRedisRequiresClient(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()
	cfg.Storage = &config.SagaStorageConfig{
		Type:  config.SagaStorageTypeRedis,
		Redis: &config.SagaRedisStorageConfig{KeysPrefix: "saga:"},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "Redis client is required")
}

func TestBuildInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultSaga()
	cfg.Storage = &config.SagaStorageConfig{Type: "bogus"}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "validate saga config")
}

func TestBuildRedis(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)

	cfg := config.DefaultSaga()
	cfg.Storage = &config.SagaStorageConfig{
		Type:  config.SagaStorageTypeRedis,
		Redis: &config.SagaRedisStorageConfig{KeysPrefix: "saga:"},
	}

	orch, err := sagafactory.New(&cfg, orderDef()).
		UseRedisClient(client).
		Build()
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}
