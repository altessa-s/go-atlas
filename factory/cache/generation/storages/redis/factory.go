// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/generation"

	redisgen "github.com/altessa-s/go-atlas/cache/generation/storages/redis"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	cachefactory "github.com/altessa-s/go-atlas/factory/cache"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var (
	// ErrClientRequired is returned when the Redis generation store is built
	// without a client.
	ErrClientRequired = corefactory.Errorf(corefactory.ErrDependencyRequired, "cache: redis client is required for generation store type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = fmt.Errorf("cache: redis configuration is required for generation store type redis: %w", corefactory.ErrConfigRequired)
)

// Builder returns the generation store function of the redis type, ready for
// cachefactory.GenerationStoreBuilder.UseStore. The store shares generations
// through client under <keysPrefix>:generation:. opts are applied after that
// derived key prefix, so an explicit redisgen.WithKeyPrefix wins.
func Builder(client redis.UniversalClient, opts ...redisgen.Option) cachefactory.GenerationStoreFunc {
	return func(cfg *storageconfig.CacheStorageConfig) (generation.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}
		all := append([]redisgen.Option{redisgen.WithKeyPrefix(keyPrefix(cfg.Redis.KeysPrefix))}, opts...)
		return redisgen.New(client, all...), nil
	}
}

// keyPrefix scopes the generation keys under the service's key prefix,
// keeping them apart from the cache entries stored under it.
func keyPrefix(keysPrefix string) string {
	if keysPrefix == "" {
		return redisgen.DefaultKeyPrefix
	}
	return strings.TrimSuffix(keysPrefix, ":") + ":" + redisgen.DefaultKeyPrefix
}
