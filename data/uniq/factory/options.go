// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *UniqBuilder) UseLogger(v *slog.Logger) *UniqBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *UniqBuilder) UseDefaultLogger() *UniqBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed providers.
func (b *UniqBuilder) UseRedisClient(v redis.UniversalClient) *UniqBuilder {
	b.redisClient = v
	return b
}

// UseNatsConn sets the NATS connection used for NATS-backed providers.
func (b *UniqBuilder) UseNatsConn(v *nats.Conn) *UniqBuilder {
	b.natsConn = v
	return b
}
