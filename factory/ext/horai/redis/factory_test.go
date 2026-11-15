// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/horai/redis"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
)

func redisConfig(ensureIndexes bool) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	storageCfg := schedulerconfig.DefaultStorageRedisConfig()
	storageCfg.EnsureIndexes = ensureIndexes
	cfg.Storage = &schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeRedis, Redis: &storageCfg}
	return &cfg
}

// build builds a scheduler with the Redis task storage registered over client.
func build(t *testing.T, cfg *schedulerconfig.Config, client redis.UniversalClient) error {
	t.Helper()
	s, err := schedulerfactory.New(cfg).
		UseStorage(schedulerconfig.StorageTypeRedis, factory.Builder(client)).
		Build(t.Context())
	if err == nil {
		require.NotNil(t, s)
	}
	return err
}

func TestBuilder(t *testing.T) {
	t.Parallel()
	errDial := errors.New("not connected")
	client := redis.NewClient(&redis.Options{
		Dialer:     func(context.Context, string, string) (net.Conn, error) { return nil, errDial },
		MaxRetries: -1,
	})
	t.Cleanup(func() { _ = client.Close() })

	t.Run("requires_client", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, build(t, redisConfig(false), nil), factory.ErrClientRequired)
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		cfg := redisConfig(false)
		cfg.Storage.Redis = nil
		require.ErrorIs(t, build(t, cfg, client), factory.ErrConfigRequired)
	})

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, build(t, redisConfig(false), client))
	})

	t.Run("ensure_indexes_failure", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, build(t, redisConfig(true), client), errDial)
	})
}
