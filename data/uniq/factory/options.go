// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
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

// UseCollector forwards a metrics collector to the resulting [uniq.Uniq].
// Without it, Uniq falls back to a no-op collector and emits no metrics.
func (b *UniqBuilder) UseCollector(v metrics.Collector) *UniqBuilder {
	b.collector = v
	return b
}

// UseHealthCoordinator registers the resulting [uniq.Uniq] with the
// supplied health coordinator on construction. Combine with
// [UniqBuilder.UseHealthServiceName] to override the default
// "uniq" service name when several Uniq instances share a coordinator.
func (b *UniqBuilder) UseHealthCoordinator(v *health.Coordinator) *UniqBuilder {
	b.healthCoordinator = v
	return b
}

// UseHealthServiceName overrides the service name used for health
// registration. Has no effect unless [UniqBuilder.UseHealthCoordinator]
// is also called.
func (b *UniqBuilder) UseHealthServiceName(v string) *UniqBuilder {
	b.healthServiceName = v
	return b
}

// UseTtl overrides the per-key TTL for the resulting [uniq.Uniq],
// taking precedence over the value supplied via the config's storage
// sub-section. Pass a positive duration; zero or negative values are
// ignored, leaving config or package default in effect.
//
// Use this when the same config feeds multiple [uniq.Uniq] instances
// that need different TTLs, or in tests that want to shrink TTL to
// keep runs fast.
func (b *UniqBuilder) UseTtl(d time.Duration) *UniqBuilder {
	if d > 0 {
		b.ttlOverride = d
	}
	return b
}
