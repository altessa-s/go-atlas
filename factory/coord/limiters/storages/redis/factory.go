// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	redisstorage "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
)

var (
	// ErrClientRequired is returned when the Redis limiter storage is built
	// without a client.
	ErrClientRequired = errors.New("limiters: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = errors.New("limiters: redis storage configuration is required")
)

// Builder returns the storage function of the redis type, ready for
// UseStorage of the budget and token-bucket limiter factories. The storage
// keeps counters through client under the configured keysPrefix.
func Builder(client redis.UniversalClient) storagesfactory.StorageFunc {
	return func(_ context.Context, cfg *storageconfig.BackendConfig, _ *limiterconfig.NATSContention) (storages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}
		return redisstorage.New(client, redisstorage.WithKeyPrefix(cfg.Redis.KeysPrefix)), nil
	}
}
