// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/cache/generation/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisgen "github.com/altessa-s/go-atlas/cache/generation/storages/redis"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	cachefactory "github.com/altessa-s/go-atlas/factory/cache"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuilder_KeyPrefix(t *testing.T) {
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
			client, mr := redistest.RedisClient(t)
			cfg := &storageconfig.CacheStorageConfig{
				Type:  storageconfig.CacheStorageTypeRedis,
				Redis: &storageconfig.RedisConfig{KeysPrefix: tc.keysPrefix},
			}

			store, err := cachefactory.NewGenerationStore(cfg).
				UseStore(storageconfig.CacheStorageTypeRedis, factory.Builder(client, tc.opts...)).
				Build()
			require.NoError(t, err)
			require.IsType(t, &redisgen.Store{}, store)

			_, err = store.Bump(t.Context(), "catalog")
			require.NoError(t, err)
			require.True(t, mr.Exists(tc.wantKey), "keys: %v", mr.Keys())
		})
	}
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(&storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis, Redis: &storageconfig.RedisConfig{}})
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	_, err = factory.Builder(client)(&storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}
