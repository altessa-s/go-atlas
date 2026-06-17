// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	goredis "github.com/redis/go-redis/v9"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

// UseLogger sets the logger for the builder and the orchestrator it builds.
func (b *Builder[T]) UseLogger(v *slog.Logger) *Builder[T] {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder[T]) UseDefaultLogger() *Builder[T] {
	return b.UseLogger(slog.Default())
}

// UseJetStream injects the NATS JetStream context used when the storage type is
// "nats".
func (b *Builder[T]) UseJetStream(v jetstream.JetStream) *Builder[T] {
	b.js = v
	return b
}

// UseMongoDatabase injects the MongoDB database used when the storage type is
// "mongo".
func (b *Builder[T]) UseMongoDatabase(v *mongodriver.Database) *Builder[T] {
	b.mongoDB = v
	return b
}

// UseRedisClient injects the Redis client used when the storage type is "redis".
func (b *Builder[T]) UseRedisClient(v goredis.UniversalClient) *Builder[T] {
	b.redisClient = v
	return b
}

// UseCollector sets the Prometheus metrics collector for the orchestrator.
func (b *Builder[T]) UseCollector(v metrics.Collector) *Builder[T] {
	b.collector = v
	return b
}

// UseScheduler registers the background recovery cycle with the given task
// registrar.
func (b *Builder[T]) UseScheduler(v corescheduler.TaskRegistrar) *Builder[T] {
	b.scheduler = v
	return b
}

// UseLeaderElector gates the recovery cycle so only the elected leader scans
// the store.
func (b *Builder[T]) UseLeaderElector(v saga.LeaderElector) *Builder[T] {
	b.leaderElector = v
	return b
}

// UseSerializer sets the codec used to (de)serialize the saga payload.
func (b *Builder[T]) UseSerializer(v serializer.Serializer) *Builder[T] {
	b.serializer = v
	return b
}

// UseOnDeadLetter registers a callback fired when a saga reaches the terminal
// Failed state.
func (b *Builder[T]) UseOnDeadLetter(v saga.DeadLetterFunc) *Builder[T] {
	b.onDeadLetter = v
	return b
}

// UseShouldRetry installs a predicate deciding whether a step or compensation
// error is retryable.
func (b *Builder[T]) UseShouldRetry(v func(error) bool) *Builder[T] {
	b.shouldRetry = v
	return b
}
