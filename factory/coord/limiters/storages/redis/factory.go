// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	redisstorage "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var (
	// ErrClientRequired is returned when the Redis limiter storage is built
	// without a client.
	ErrClientRequired = errors.New("limiters: redis client is required for storage type redis")
	// ErrConfigRequired is returned when the storage configuration has no
	// redis block.
	ErrConfigRequired = fmt.Errorf("limiters: redis storage configuration is required: %w", corefactory.ErrConfigRequired)
)

// Builder returns the storage function of the redis type, ready for
// UseStorage of the budget and token-bucket limiter factories. The storage
// keeps counters through client under the configured keysPrefix, or
// [redisstorage.DefaultKeyPrefix] ("ratelimit:") when keysPrefix is empty.
func Builder(client redis.UniversalClient) storagesfactory.StorageFunc {
	return func(_ context.Context, cfg *limiterconfig.StorageConfig) (storages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if cfg.Redis == nil {
			return nil, ErrConfigRequired
		}
		opts := coreslices.AppendIfFunc(nil, strings.TrimSpace(cfg.Redis.KeysPrefix) != "", func() []redisstorage.Option {
			return []redisstorage.Option{redisstorage.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return redisstorage.New(client, opts...), nil
	}
}
