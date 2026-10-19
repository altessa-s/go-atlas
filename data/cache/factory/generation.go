// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/cache/generation"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorygen "github.com/altessa-s/go-atlas/data/cache/generation/storages/memory"
	redisgen "github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
)

// GenerationStoreBuilder assembles a cache [generation.Store] from the same
// [storageconfig.CacheStorageConfig] that selects the cache storage, so a
// service configures one `storage` block for both. Create instances with
// [NewGenerationStore]. The builder is not safe for concurrent use.
type GenerationStoreBuilder struct {
	corefactory.Base
	cfg *storageconfig.CacheStorageConfig

	// Dependencies
	redisClient  redis.UniversalClient
	redisOptions []redisgen.Option
}

// NewGenerationStore creates a [GenerationStoreBuilder] for the given cache
// storage config. A nil config or type memory builds an in-process store
// (not shared between replicas); type redis builds a Redis store with the
// config's keysPrefix prepended to the default generation prefix.
func NewGenerationStore(cfg *storageconfig.CacheStorageConfig) *GenerationStoreBuilder {
	return &GenerationStoreBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// UseLogger sets the logger for the builder.
func (b *GenerationStoreBuilder) UseLogger(v *slog.Logger) *GenerationStoreBuilder {
	b.SetLogger(v)
	return b
}

// UseRedisClient sets the Redis client of a Redis-backed store.
func (b *GenerationStoreBuilder) UseRedisClient(v redis.UniversalClient) *GenerationStoreBuilder {
	b.redisClient = v
	return b
}

// UseRedisOptions appends options of a Redis-backed store (for example
// redisgen.WithMaxStaleness). They are applied after the key prefix derived
// from the config, so an explicit redisgen.WithKeyPrefix wins.
func (b *GenerationStoreBuilder) UseRedisOptions(opts ...redisgen.Option) *GenerationStoreBuilder {
	b.redisOptions = append(b.redisOptions, opts...)
	return b
}

// Build assembles the generation store.
func (b *GenerationStoreBuilder) Build() (generation.Store, error) {
	if b.cfg == nil {
		return memorygen.New(), nil
	}

	switch b.cfg.Type {
	case storageconfig.CacheStorageTypeMemory:
		return memorygen.New(), nil
	case storageconfig.CacheStorageTypeRedis:
		if b.cfg.Redis == nil {
			return nil, b.Errorf("redis configuration is required")
		}
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		opts := []redisgen.Option{redisgen.WithKeyPrefix(generationKeyPrefix(b.cfg.Redis.KeysPrefix))}
		return redisgen.New(b.redisClient, append(opts, b.redisOptions...)...), nil
	default:
		return nil, b.Errorf("unsupported generation store type: %s", b.cfg.Type)
	}
}

// generationKeyPrefix scopes the generation keys under the service's key
// prefix, keeping them apart from the cache entries stored under it.
func generationKeyPrefix(keysPrefix string) string {
	if keysPrefix == "" {
		return redisgen.DefaultKeyPrefix
	}
	return strings.TrimSuffix(keysPrefix, ":") + ":" + redisgen.DefaultKeyPrefix
}
