// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	redisstore "github.com/altessa-s/go-atlas/reliability/saga/storages/redis"
)

var (
	// ErrClientRequired is returned when the Redis saga store is built
	// without a client.
	ErrClientRequired = errors.New("saga: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = fmt.Errorf("saga: redis configuration is required for storage type redis: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the redis type, ready for
// sagafactory.Builder.UseStorage. The store keeps instances through client
// under the configured keysPrefix with the configured TTL.
func Builder(client redis.UniversalClient) sagafactory.StorageFunc {
	return func(_ context.Context, cfg *sagaconfig.StorageConfig) (saga.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}
		return redisstore.New(client,
			redisstore.WithKeyPrefix(cfg.Redis.KeysPrefix),
			redisstore.WithTTL(cfg.Redis.TTL),
		)
	}
}
