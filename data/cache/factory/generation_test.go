// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	memorygen "github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
	redisgen "github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
)

func TestNewGenerationStore_Memory(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*storageconfig.CacheStorageConfig{nil, {Type: storageconfig.CacheStorageTypeMemory}} {
		store, err := factory.NewGenerationStore(cfg).Build()
		require.NoError(t, err)
		require.IsType(t, &memorygen.Store{}, store)
	}
}

func TestNewGenerationStore_Redis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		keysPrefix string
		opts       []redisgen.Option
		wantKey    string
	}{
		{name: "no service prefix", wantKey: "generation:catalog"},
		{name: "service prefix", keysPrefix: "svc", wantKey: "svc:generation:catalog"},
		{name: "service prefix with separator", keysPrefix: "svc:", wantKey: "svc:generation:catalog"},
		{name: "explicit prefix wins", keysPrefix: "svc", opts: []redisgen.Option{redisgen.WithKeyPrefix("own")}, wantKey: "own:catalog"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := testhelpers.RedisClient(t)
			cfg := &storageconfig.CacheStorageConfig{
				Type:  storageconfig.CacheStorageTypeRedis,
				Redis: &storageconfig.RedisConfig{KeysPrefix: tc.keysPrefix},
			}

			store, err := factory.NewGenerationStore(cfg).UseRedisClient(client).UseRedisOptions(tc.opts...).Build()
			require.NoError(t, err)
			require.IsType(t, &redisgen.Store{}, store)

			_, err = store.Bump(t.Context(), "catalog")
			require.NoError(t, err)
			require.True(t, mr.Exists(tc.wantKey), "keys: %v", mr.Keys())
		})
	}
}

func TestNewGenerationStore_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.NewGenerationStore(&storageconfig.CacheStorageConfig{
		Type:  storageconfig.CacheStorageTypeRedis,
		Redis: &storageconfig.RedisConfig{},
	}).Build()
	require.Error(t, err, "redis without a client")

	_, err = factory.NewGenerationStore(&storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}).Build()
	require.Error(t, err, "redis without its config block")

	_, err = factory.NewGenerationStore(&storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeNATS}).Build()
	require.Error(t, err, "unsupported type")
}
