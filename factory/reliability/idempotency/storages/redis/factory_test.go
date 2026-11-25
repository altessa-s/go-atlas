// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	idempotencyfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
)

func redisCfg(rc *storageconfig.RedisConfig) *idempotencyconfig.Config {
	return &idempotencyconfig.Config{TTL: time.Hour, MaxLockDuration: time.Minute, Storage: &idempotencyconfig.StorageConfig{
		Type: idempotencyconfig.StorageTypeRedis, Redis: rc,
	}}
}

func TestBuilder_UsesPrefix(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)

	keeper, err := idempotencyfactory.New(redisCfg(&storageconfig.RedisConfig{KeysPrefix: "idem:"})).
		UseDefaultLogger().
		UseStorage(idempotencyconfig.StorageTypeRedis, factory.Builder(client)).
		Build(t.Context())
	require.NoError(t, err)

	ok, _, err := keeper.AttemptLock(t.Context(), "order-1")
	require.NoError(t, err)
	require.True(t, ok)

	keys := mr.Keys()
	require.Len(t, keys, 1)
	require.Contains(t, keys[0], "idem:")
	require.Contains(t, keys[0], "order-1")
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(redisCfg(&storageconfig.RedisConfig{}))
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	_, err = factory.Builder(client)(redisCfg(nil))
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}
