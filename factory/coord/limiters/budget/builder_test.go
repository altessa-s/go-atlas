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
)

var errBuild = errors.New("build failed")

func validConfig() *limiterconfig.Budget {
	return &limiterconfig.Budget{
		Limit:  1000,
		Period: time.Hour,
		Storage: &storageconfig.BackendConfig{
			Type:   storageconfig.BackendTypeMemory,
			Memory: &storageconfig.MemoryConfig{},
		},
	}
}

func TestBuild_NilConfig(t *testing.T) {
	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err, "Build() with nil config should return error")
}

func TestBuild_NilStorage(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = nil

	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err, "Build() with nil storage should return error")
}

func TestBuild_MemoryStorage(t *testing.T) {
	l, err := factory.New(validConfig()).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}

func TestBuild_DriverStorageNotRegistered(t *testing.T) {
	t.Parallel()

	for _, typ := range []storageconfig.BackendType{storageconfig.BackendTypeRedis, storageconfig.BackendTypeNATS} {
		cfg := validConfig()
		cfg.Storage = &storageconfig.BackendConfig{Type: typ}

		_, err := factory.New(cfg).Build(t.Context())
		require.Error(t, err, "Build() with an unregistered %s storage should return error", typ)
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Storage = &storageconfig.BackendConfig{Type: storageconfig.BackendTypeRedis}
	cfg.NATSContention = &limiterconfig.NATSContention{MaxCASAttempts: 3}

	_, err := factory.New(cfg).UseStorage(storageconfig.BackendTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *storageconfig.BackendConfig, *limiterconfig.NATSContention) (storages.Storage, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).UseStorage(storageconfig.BackendTypeRedis, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	var called bool
	registered := func(ctx context.Context, got *storageconfig.BackendConfig, contention *limiterconfig.NATSContention) (storages.Storage, error) {
		called = true
		require.NotNil(t, ctx)
		require.Same(t, cfg.Storage, got)
		require.Same(t, cfg.NATSContention, contention)
		return memorystorage.New(), nil
	}
	l, err := factory.New(cfg).
		UseStorage(storageconfig.BackendTypeRedis, failing).
		UseStorage(storageconfig.BackendTypeRedis, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l)
	require.True(t, called, "the registered storage function must build the storage")
}

func TestBuild_UnsupportedStorageType(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = &storageconfig.BackendConfig{Type: "unknown"}

	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err, "Build() with unsupported storage type should return error")
}

func TestBuild_WithLogger(t *testing.T) {
	l, err := factory.New(validConfig()).
		UseDefaultLogger().
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}
