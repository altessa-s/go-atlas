// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/storages"
	"github.com/altessa-s/go-atlas/factory/cache"

	freecacheprovider "github.com/altessa-s/go-atlas/cache/storages/freecache"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

var errBuild = errors.New("build failed")

func TestNew_MemoryStorage(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*storageconfig.CacheStorageConfig{nil, {Type: storageconfig.CacheStorageTypeMemory}} {
		storage, err := factory.New(cfg).UseDefaultLogger().Build()
		require.NoError(t, err)
		require.IsType(t, &freecacheprovider.Storage{}, storage)

		require.NoError(t, storage.Save(t.Context(), "k", []byte("v"), time.Minute))
		got, err := storage.Get(t.Context(), "k")
		require.NoError(t, err)
		require.Equal(t, []byte("v"), got)
	}
}

func TestNew_BuildErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *storageconfig.CacheStorageConfig
	}{
		{name: "redis not registered", cfg: &storageconfig.CacheStorageConfig{
			Type: storageconfig.CacheStorageTypeRedis, Redis: &storageconfig.RedisConfig{},
		}},
		{name: "unsupported type", cfg: &storageconfig.CacheStorageConfig{Type: "nats"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage, err := factory.New(tc.cfg).Build()
			require.Error(t, err)
			require.Nil(t, storage)
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := &storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}

	_, err := factory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, nil).Build()
	require.Error(t, err)

	failing := func(*storageconfig.CacheStorageConfig) (storages.Storage, error) { return nil, errBuild }
	_, err = factory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, failing).Build()
	require.ErrorIs(t, err, errBuild)

	want := freecacheprovider.New()
	registered := func(got *storageconfig.CacheStorageConfig) (storages.Storage, error) {
		require.Same(t, cfg, got)
		return want, nil
	}
	storage, err := factory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, registered).Build()
	require.NoError(t, err)
	require.Same(t, want, storage)
}
