// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *TokenBucketLimiterBuilder) UseLogger(v *slog.Logger) *TokenBucketLimiterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *TokenBucketLimiterBuilder) UseDefaultLogger() *TokenBucketLimiterBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client for Redis storage backends.
func (b *TokenBucketLimiterBuilder) UseRedisClient(v redis.UniversalClient) *TokenBucketLimiterBuilder {
	b.redisClient = v
	return b
}

// UseJetstream sets the NATS JetStream context for NATS storage backends.
func (b *TokenBucketLimiterBuilder) UseJetstream(v jetstream.JetStream) *TokenBucketLimiterBuilder {
	b.jetstream = v
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *TokenBucketLimiterBuilder) UseScheduler(v corescheduler.TaskRegistrar) *TokenBucketLimiterBuilder {
	b.scheduler = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording rate limiter metrics.
func (b *TokenBucketLimiterBuilder) UseCollector(v metrics.Collector) *TokenBucketLimiterBuilder {
	b.collector = v
	return b
}

// UseClientService sets the [tokenbucket.ClientService] used to resolve
// per-client rate limits from authenticated tokens. When unset, the limiter
// falls back to IP-based rules only.
func (b *TokenBucketLimiterBuilder) UseClientService(v tokenbucket.ClientService) *TokenBucketLimiterBuilder {
	b.clientService = v
	return b
}
