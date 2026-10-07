// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/cache/storages"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	freecacheprovider "github.com/altessa-s/go-atlas/data/cache/storages/freecache"
	redisprovider "github.com/altessa-s/go-atlas/data/cache/storages/redis"
)

// StorageBuilder assembles a cache [storages.Storage] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [StorageBuilder.Build] time.
// The builder is not safe for concurrent use.
type StorageBuilder struct {
	corefactory.Base
	cfg  *storageconfig.CacheStorageConfig
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
}

// New creates a [StorageBuilder] for the given cache storage config.
// Config can be nil — the builder returns a FreeCache (memory) provider if nil.
func New(cfg *storageconfig.CacheStorageConfig) *StorageBuilder {
	return &StorageBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the cache provider. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *StorageBuilder) Build() (storages.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return b.createFreeCacheStorage(), nil
	}

	switch b.cfg.Type {
	case storageconfig.CacheStorageTypeMemory:
		return b.createFreeCacheStorage(), nil
	case storageconfig.CacheStorageTypeRedis:
		if b.cfg.Redis == nil {
			return nil, fmt.Errorf("configuration is required")
		}
		return b.createRedisStorageFromConfig()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Type)
	}
}

// createFreeCacheStorage creates an in-memory FreeCache provider.
func (b *StorageBuilder) createFreeCacheStorage() *freecacheprovider.Storage {
	return freecacheprovider.New()
}

// createRedisStorageFromConfig creates a Redis cache provider from configuration.
func (b *StorageBuilder) createRedisStorageFromConfig() (*redisprovider.Storage, error) {
	if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
		return nil, err
	}
	return redisprovider.New(b.redisClient, redisprovider.WithPrefix(b.cfg.Redis.KeysPrefix)), nil
}
