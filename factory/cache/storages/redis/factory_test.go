// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/cache/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisprovider "github.com/altessa-s/go-atlas/cache/storages/redis"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	cachefactory "github.com/altessa-s/go-atlas/factory/cache"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuilder_UsesClientAndPrefix(t *testing.T) {
	t.Parallel()

	client, mr := redistest.RedisClient(t)
	cfg := &storageconfig.CacheStorageConfig{
		Type:  storageconfig.CacheStorageTypeRedis,
		Redis: &storageconfig.RedisConfig{KeysPrefix: "svc"},
	}

	storage, err := cachefactory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, factory.Builder(client)).Build()
	require.NoError(t, err)
	require.IsType(t, &redisprovider.Storage{}, storage)

	require.NoError(t, storage.Save(t.Context(), "k", []byte("v"), time.Minute))
	keys := mr.Keys()
	require.Len(t, keys, 1)
	require.True(t, strings.HasPrefix(keys[0], "svc"), "key %q must carry the configured prefix", keys[0])

	got, err := storage.Get(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)
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
