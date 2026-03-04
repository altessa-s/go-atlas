// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/cache/providers"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	freecacheprovider "github.com/altessa-s/go-atlas/data/cache/providers/freecache"
	redisprovider "github.com/altessa-s/go-atlas/data/cache/providers/redis"
)

// ProviderBuilder assembles a cache [providers.Provider] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ProviderBuilder.Build] time.
// The builder is not safe for concurrent use.
type ProviderBuilder struct {
	corefactory.Base
	cfg  *config.CacheStorageConfig
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
}

// New creates a [ProviderBuilder] for the given cache storage config.
// Config can be nil — the builder returns a FreeCache (memory) provider if nil.
func New(cfg *config.CacheStorageConfig) *ProviderBuilder {
	return &ProviderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the cache provider. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ProviderBuilder) Build() (providers.Provider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return b.createFreeCacheProvider(), nil
	}

	switch b.cfg.Type {
	case config.CacheStorageTypeMemory:
		return b.createFreeCacheProvider(), nil
	case config.CacheStorageTypeRedis:
		if b.cfg.Redis == nil {
			return nil, fmt.Errorf("configuration is required")
		}
		return b.createRedisProviderFromConfig()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Type)
	}
}

// createFreeCacheProvider creates an in-memory FreeCache provider.
func (b *ProviderBuilder) createFreeCacheProvider() *freecacheprovider.Provider {
	return freecacheprovider.New()
}

// createRedisProviderFromConfig creates a Redis cache provider from configuration.
func (b *ProviderBuilder) createRedisProviderFromConfig() (*redisprovider.Provider, error) {
	if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
		return nil, err
	}
	return redisprovider.New(b.redisClient, redisprovider.WithPrefix(b.cfg.Redis.KeysPrefix)), nil
}
