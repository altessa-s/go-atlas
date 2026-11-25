// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	idempotencyfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	redisstorage "github.com/altessa-s/go-atlas/reliability/idempotency/storages/redis"
)

var (
	// ErrClientRequired is returned when the Redis idempotency storage is
	// built without a client.
	ErrClientRequired = errors.New("idempotency: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = fmt.Errorf("idempotency: redis configuration is required for storage type redis: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the redis type, ready for
// idempotencyfactory.KeeperBuilder.UseStorage. The storage keeps keys through
// client under the configured keysPrefix, expiring them after the configured TTL.
func Builder(client redis.UniversalClient) idempotencyfactory.StorageFunc {
	return func(cfg *idempotencyconfig.Config) (storages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Storage == nil || cfg.Storage.Redis == nil {
			return nil, ErrConfigRequired
		}
		return redisstorage.New(client,
			redisstorage.WithTTL(cfg.TTL),
			redisstorage.WithKeyPrefix(cfg.Storage.Redis.KeysPrefix),
		), nil
	}
}
