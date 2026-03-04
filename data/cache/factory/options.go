// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ProviderBuilder) UseLogger(v *slog.Logger) *ProviderBuilder {
	if v != nil {
		b.Base = corefactory.NewBase(v)
	}
	return b
}

// UseRedisClient sets the Redis client used for Redis-backed cache providers.
func (b *ProviderBuilder) UseRedisClient(v redis.UniversalClient) *ProviderBuilder {
	b.redisClient = v
	return b
}
