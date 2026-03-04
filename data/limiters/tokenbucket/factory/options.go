// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *LimiterBuilder) UseLogger(v *slog.Logger) *LimiterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *LimiterBuilder) UseDefaultLogger() *LimiterBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client for Redis storage backends.
func (b *LimiterBuilder) UseRedisClient(v redis.UniversalClient) *LimiterBuilder {
	b.redisClient = v
	return b
}

// UseJetstream sets the NATS JetStream context for NATS storage backends.
func (b *LimiterBuilder) UseJetstream(v jetstream.JetStream) *LimiterBuilder {
	b.jetstream = v
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *LimiterBuilder) UseScheduler(v corescheduler.TaskRegistrar) *LimiterBuilder {
	b.scheduler = v
	return b
}
