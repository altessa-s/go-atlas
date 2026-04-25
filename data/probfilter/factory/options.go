// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// --- FilterBuilder dependency methods ---

// UseLogger sets the logger for the filter builder and all created components.
func (b *FilterBuilder) UseLogger(v *slog.Logger) *FilterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *FilterBuilder) UseDefaultLogger() *FilterBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed filter storages.
func (b *FilterBuilder) UseRedisClient(v redis.UniversalClient) *FilterBuilder {
	b.redisClient = v
	return b
}

// --- ManagerBuilder dependency methods ---

// UseLogger sets the logger for the manager builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder) UseDefaultLogger() *ManagerBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed filter storages.
func (b *ManagerBuilder) UseRedisClient(v redis.UniversalClient) *ManagerBuilder {
	b.redisClient = v
	return b
}
