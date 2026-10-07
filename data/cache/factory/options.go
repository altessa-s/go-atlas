// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *StorageBuilder) UseLogger(v *slog.Logger) *StorageBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *StorageBuilder) UseDefaultLogger() *StorageBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed cache storages.
func (b *StorageBuilder) UseRedisClient(v redis.UniversalClient) *StorageBuilder {
	b.redisClient = v
	return b
}
