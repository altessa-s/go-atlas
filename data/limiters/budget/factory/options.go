// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *BudgetLimiterBuilder) UseLogger(v *slog.Logger) *BudgetLimiterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *BudgetLimiterBuilder) UseDefaultLogger() *BudgetLimiterBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client for Redis storage backends.
func (b *BudgetLimiterBuilder) UseRedisClient(v redis.UniversalClient) *BudgetLimiterBuilder {
	b.redisClient = v
	return b
}

// UseJetstream sets the NATS JetStream context for NATS storage backends.
func (b *BudgetLimiterBuilder) UseJetstream(v jetstream.JetStream) *BudgetLimiterBuilder {
	b.jetstream = v
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *BudgetLimiterBuilder) UseScheduler(v corescheduler.TaskRegistrar) *BudgetLimiterBuilder {
	b.scheduler = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording budget limiter metrics.
func (b *BudgetLimiterBuilder) UseCollector(v metrics.Collector) *BudgetLimiterBuilder {
	b.collector = v
	return b
}
