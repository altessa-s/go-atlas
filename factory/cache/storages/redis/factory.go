// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/storages"

	redisprovider "github.com/altessa-s/go-atlas/cache/storages/redis"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	cachefactory "github.com/altessa-s/go-atlas/factory/cache"
)

var (
	// ErrClientRequired is returned when the Redis cache storage is built
	// without a client.
	ErrClientRequired = errors.New("cache: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = errors.New("cache: redis configuration is required for storage type redis")
)

// Builder returns the storage function of the redis type, ready for
// cachefactory.StorageBuilder.UseStorage. The storage keeps entries through
// client under the configured keysPrefix.
func Builder(client redis.UniversalClient) cachefactory.StorageFunc {
	return func(cfg *storageconfig.CacheStorageConfig) (storages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}
		return redisprovider.New(client, redisprovider.WithPrefix(cfg.Redis.KeysPrefix)), nil
	}
}
