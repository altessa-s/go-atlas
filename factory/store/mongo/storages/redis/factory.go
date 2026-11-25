// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/store/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	mongofactory "github.com/altessa-s/go-atlas/factory/store/mongo"
	redisstorage "github.com/altessa-s/go-atlas/store/mongo/storages/redis"
)

var (
	// ErrClientRequired is returned when the Redis cursor storage is built
	// without a client.
	ErrClientRequired = errors.New("cursor storage: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = fmt.Errorf("cursor storage: redis configuration is required for storage type redis: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the redis type, ready for
// mongofactory.CursorStorageBuilder.UseStorage. The storage keeps cursors
// through client under the configured keysPrefix with the builder's TTL.
func Builder(client redis.UniversalClient) mongofactory.StorageFunc {
	return func(_ context.Context, cfg *storageconfig.BackendConfig, ttl time.Duration, _ *slog.Logger) (mongo.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}

		opts := []redisstorage.Option{
			redisstorage.WithKeyPrefix(cfg.Redis.KeysPrefix),
			redisstorage.WithTTL(ttl),
		}

		return redisstorage.New(client, opts...), nil
	}
}
