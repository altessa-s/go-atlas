// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// UseLogger sets the logger for the builder and all created components.
func (b *SchedulerBuilder) UseLogger(v *slog.Logger) *SchedulerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *SchedulerBuilder) UseDefaultLogger() *SchedulerBuilder {
	return b.UseLogger(slog.Default())
}

// UseLeaderElector sets the leader elector for distributed scheduling.
func (b *SchedulerBuilder) UseLeaderElector(v leadelect.LeaderElector) *SchedulerBuilder {
	b.leaderElector = v
	return b
}

// UseMongoDb sets the MongoDB database for MongoDB storage backends.
func (b *SchedulerBuilder) UseMongoDb(v *mongo.Database) *SchedulerBuilder {
	b.mongoDb = v
	return b
}

// UseRedisClient sets the Redis client for Redis storage backends.
func (b *SchedulerBuilder) UseRedisClient(v redis.UniversalClient) *SchedulerBuilder {
	b.redisClient = v
	return b
}

// UseCollector sets the [metrics.Collector] for recording scheduler metrics.
func (b *SchedulerBuilder) UseCollector(v metrics.Collector) *SchedulerBuilder {
	b.collector = v
	return b
}
