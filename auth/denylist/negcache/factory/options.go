// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// UseLogger sets the logger for the builder and the filter it constructs.
func (b *Builder) UseLogger(v *slog.Logger) *Builder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder) UseDefaultLogger() *Builder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client passed through to the probfilter factory
// for Redis-backed negative-filter storage. It is required only when the filter
// config selects the redis storage type; in-memory filters need no client. The
// factory never constructs the client — the caller injects it.
func (b *Builder) UseRedisClient(v redis.UniversalClient) *Builder {
	b.redisClient = v
	return b
}

// UseMetrics enables lookup telemetry on the built cache using the given
// collector and metric subsystem. A nil collector leaves metrics disabled; an
// empty subsystem falls back to [negcache.DefaultMetricsSubsystem].
func (b *Builder) UseMetrics(collector metrics.Collector, subsystem string) *Builder {
	b.metricsCollector = collector
	b.metricsSubsystem = subsystem
	return b
}
