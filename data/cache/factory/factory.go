// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of cache instances.
package factory

import (
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/providers"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	freecacheprovider "github.com/altessa-s/go-atlas/data/cache/providers/freecache"
	noopprovider "github.com/altessa-s/go-atlas/data/cache/providers/noop"
	redisprovider "github.com/altessa-s/go-atlas/data/cache/providers/redis"
)

// Factory creates cache instances and providers from configuration.
type Factory struct {
	corefactory.Base
	redisClient redis.UniversalClient
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		redisClient: cfg.redisClient,
	}
}

// CreateNoopCache creates a Cache with a no-op provider.
func (f *Factory) CreateNoopCache() *cache.Cache {
	return cache.NewNoop()
}

// CreateNoopProvider creates a no-op cache provider.
func (f *Factory) CreateNoopProvider() *noopprovider.Provider {
	return noopprovider.New()
}

// CreateProviderFromConfig creates a cache provider based on configuration.
// Returns a FreeCache provider if cfg is nil or type is memory.
// Panics if cfg.Redis is nil when type is redis.
func (f *Factory) CreateProviderFromConfig(cfg *config.CacheStorageConfig) (providers.Provider, error) {
	if cfg == nil {
		return f.createFreeCacheProvider(), nil
	}

	switch cfg.Type {
	case config.CacheStorageTypeMemory:
		return f.createFreeCacheProvider(), nil
	case config.CacheStorageTypeRedis:
		if cfg.Redis == nil {
			return nil, fmt.Errorf("configuration is required")
		}
		return f.createRedisProviderFromConfig(cfg.Redis)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// createFreeCacheProvider creates an in-memory FreeCache provider.
func (f *Factory) createFreeCacheProvider() *freecacheprovider.Provider {
	return freecacheprovider.New()
}

// createRedisProviderFromConfig creates a Redis cache provider from configuration.
func (f *Factory) createRedisProviderFromConfig(cfg *config.StorageRedisConfig) (*redisprovider.Provider, error) {
	if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
		return nil, err
	}
	return redisprovider.New(f.redisClient, redisprovider.WithPrefix(cfg.KeysPrefix)), nil
}
