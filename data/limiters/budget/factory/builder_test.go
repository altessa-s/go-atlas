// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/limiters/budget/factory"
)

func validConfig() *config.BudgetLimiter {
	return &config.BudgetLimiter{
		Limit:  1000,
		Period: time.Hour,
		Storage: &config.CacheStorageConfig{
			Type:   config.CacheStorageTypeMemory,
			Memory: &config.StorageMemoryConfig{},
		},
	}
}

func TestBuild_NilConfig(t *testing.T) {
	_, err := factory.New(nil).Build()
	require.Error(t, err, "Build() with nil config should return error")
}

func TestBuild_NilStorage(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = nil

	_, err := factory.New(cfg).Build()
	require.Error(t, err, "Build() with nil storage should return error")
}

func TestBuild_MemoryStorage(t *testing.T) {
	l, err := factory.New(validConfig()).Build()
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}

func TestBuild_RedisStorage_NoDependency(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = &config.CacheStorageConfig{
		Type:  config.CacheStorageTypeRedis,
		Redis: &config.StorageRedisConfig{},
	}

	_, err := factory.New(cfg).Build()
	require.Error(t, err, "Build() without redis client should return error")
}

func TestBuild_NatsStorage_NoDependency(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = &config.CacheStorageConfig{
		Type: config.CacheStorageTypeNats,
		Nats: &config.StorageNATSConfig{},
	}

	_, err := factory.New(cfg).Build()
	require.Error(t, err, "Build() without jetstream should return error")
}

func TestBuild_UnsupportedStorageType(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = &config.CacheStorageConfig{Type: "unknown"}

	_, err := factory.New(cfg).Build()
	require.Error(t, err, "Build() with unsupported storage type should return error")
}

func TestBuild_WithLogger(t *testing.T) {
	l, err := factory.New(validConfig()).
		UseDefaultLogger().
		Build()
	require.NoError(t, err)
	require.NotNil(t, l, "Build() returned nil")
}
