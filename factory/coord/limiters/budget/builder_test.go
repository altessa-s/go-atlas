// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/factory/coord/limiters/budget"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	memorystorage "github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var errBuild = errors.New("build failed")

func validConfig() *limiterconfig.Budget {
	return &limiterconfig.Budget{
		Limit:  1000,
		Period: time.Hour,
		Storage: &limiterconfig.StorageConfig{
			Type:   limiterconfig.StorageTypeMemory,
			Memory: &storageconfig.MemoryConfig{},
		},
	}
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err, "Build() with nil config should return error")
}

func TestBuild_NilStorage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Storage = nil

	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err, "Build() with nil storage should return error")
}

func TestBuild_MemoryStorage(t *testing.T) {
	t.Parallel()
	l, err := factory.New(validConfig()).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}

func TestBuild_DriverStorageNotRegistered(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Storage = &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}

	_, err := factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrNotRegistered)
}

func TestBuild_NATSStorageUnsupported(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Storage = &limiterconfig.StorageConfig{Type: "nats"}

	_, err := factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Storage = &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}

	_, err := factory.New(cfg).UseStorage(limiterconfig.StorageTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *limiterconfig.StorageConfig) (storages.Storage, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).UseStorage(limiterconfig.StorageTypeRedis, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	var called bool
	registered := func(ctx context.Context, got *limiterconfig.StorageConfig) (storages.Storage, error) {
		called = true
		require.NotNil(t, ctx)
		require.Same(t, cfg.Storage, got)
		return memorystorage.New(), nil
	}
	l, err := factory.New(cfg).
		UseStorage(limiterconfig.StorageTypeRedis, failing).
		UseStorage(limiterconfig.StorageTypeRedis, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l)
	require.True(t, called, "the registered storage function must build the storage")
}

func TestBuild_UnsupportedStorageType(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Storage = &limiterconfig.StorageConfig{Type: "unknown"}

	_, err := factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestBuild_WithLogger(t *testing.T) {
	t.Parallel()
	l, err := factory.New(validConfig()).
		UseDefaultLogger().
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Storage = &limiterconfig.StorageConfig{Type: "custom"}

	build := func(context.Context, *limiterconfig.StorageConfig) (storages.Storage, error) {
		return memorystorage.New(), nil
	}
	b := factory.New(cfg).UseStorage("custom", build)
	cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	require.NoError(t, cfg.Storage.Validate())
	got, err := b.Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)
	_, err = factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}
