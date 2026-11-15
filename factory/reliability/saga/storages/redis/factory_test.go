// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/reliability/saga/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
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

func TestBuilder(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)

	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeRedis,
		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
	}

	orch, err := sagafactory.New(&cfg, orderDef()).
		UseStorage(sagaconfig.StorageTypeRedis, factory.Builder(client)).
		Build(t.Context())
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	storage := &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeRedis,
		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
	}
	_, err := factory.Builder(nil)(t.Context(), storage)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	_, err = factory.Builder(client)(t.Context(), &sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeRedis})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}
