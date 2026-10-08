// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *KeeperBuilder) UseLogger(v *slog.Logger) *KeeperBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *KeeperBuilder) UseDefaultLogger() *KeeperBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client for Redis storage backends.
func (b *KeeperBuilder) UseRedisClient(v redis.UniversalClient) *KeeperBuilder {
	b.redisClient = v
	return b
}

// UseJetstream sets the NATS JetStream context for NATS storage backends.
func (b *KeeperBuilder) UseJetstream(v jetstream.JetStream) *KeeperBuilder {
	b.jetstream = v
	return b
}

// UseMongoDatabase sets the MongoDB database for the mongo storage backend.
func (b *KeeperBuilder) UseMongoDatabase(v *mongo.Database) *KeeperBuilder {
	b.mongoDatabase = v
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *KeeperBuilder) UseScheduler(v corescheduler.TaskRegistrar) *KeeperBuilder {
	b.scheduler = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording idempotency metrics.
func (b *KeeperBuilder) UseCollector(v metrics.Collector) *KeeperBuilder {
	b.collector = v
	return b
}
