// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/generation"
	"github.com/altessa-s/go-atlas/factory/cache"

	memorygen "github.com/altessa-s/go-atlas/cache/generation/storages/memory"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

func TestNewGenerationStore_Memory(t *testing.T) {
	t.Parallel()

	for _, cfg := range []*storageconfig.CacheStorageConfig{nil, {Type: storageconfig.CacheStorageTypeMemory}} {
		store, err := factory.NewGenerationStore(cfg).Build()
		require.NoError(t, err)
		require.IsType(t, &memorygen.Store{}, store)
	}
}

func TestNewGenerationStore_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.NewGenerationStore(&storageconfig.CacheStorageConfig{
		Type:  storageconfig.CacheStorageTypeRedis,
		Redis: &storageconfig.RedisConfig{},
	}).Build()
	require.Error(t, err, "redis not registered")

	_, err = factory.NewGenerationStore(&storageconfig.CacheStorageConfig{Type: "nats"}).Build()
	require.Error(t, err, "unsupported type")
}

func TestNewGenerationStore_UseStore(t *testing.T) {
	t.Parallel()
	cfg := &storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}

	_, err := factory.NewGenerationStore(cfg).UseStore(storageconfig.CacheStorageTypeRedis, nil).Build()
	require.Error(t, err)

	failing := func(*storageconfig.CacheStorageConfig) (generation.Storage, error) { return nil, errBuild }
	_, err = factory.NewGenerationStore(cfg).UseStore(storageconfig.CacheStorageTypeRedis, failing).Build()
	require.ErrorIs(t, err, errBuild)

	want := memorygen.New()
	store, err := factory.NewGenerationStore(cfg).
		UseLogger(slog.Default()).
		UseStore(storageconfig.CacheStorageTypeRedis, func(*storageconfig.CacheStorageConfig) (generation.Storage, error) { return want, nil }).
		Build()
	require.NoError(t, err)
	require.Same(t, want, store)
}
