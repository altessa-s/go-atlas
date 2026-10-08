// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/observability/metrics"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	memorystorage "github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/data/idempotency/storages/mongo"
	natsstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

// KeeperBuilder assembles an idempotency [idempotency.Keeper] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [KeeperBuilder.Build] time.
// The builder is not safe for concurrent use.
type KeeperBuilder struct {
	corefactory.Base
	cfg  *idempotencyconfig.Config
	errs []error

	// Dependencies
	redisClient   redis.UniversalClient
	jetstream     jetstream.JetStream
	mongoDatabase *mongo.Database
	scheduler     corescheduler.TaskRegistrar
	collector     metrics.Collector
}

// New creates a [KeeperBuilder] for the given idempotency config.
// Config can be nil — the error surfaces at [KeeperBuilder.Build] time.
func New(cfg *idempotencyconfig.Config) *KeeperBuilder {
	return &KeeperBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the idempotency Keeper. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *KeeperBuilder) Build() (*idempotency.Keeper, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := b.createStorage()
	if err != nil {
		return nil, err
	}

	return b.createKeeper(storage), nil
}

// createKeeper creates a Keeper with the given storage.
func (b *KeeperBuilder) createKeeper(storage storages.Storage) *idempotency.Keeper {
	opts := []idempotency.Option{
		idempotency.WithLogger(b.Logger()),
		idempotency.WithCollector(b.collector),
	}
	opts = coreslices.AppendIf(opts, b.cfg.MaxLockDuration > 0,
		idempotency.WithMaxLockDuration(b.cfg.MaxLockDuration))
	return idempotency.New(storage, opts...)
}

// createStorage creates a storage backend based on configuration.
// The ttl parameter is applied to the storage backend for key expiration.
func (b *KeeperBuilder) createStorage() (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Storage.Type {
	case idempotencyconfig.StorageTypeMemory:
		return b.createMemoryStorage()
	case idempotencyconfig.StorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case idempotencyconfig.StorageTypeNATS:
		if err := b.RequireDependency(b.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return b.createNatsStorage()
	case idempotencyconfig.StorageTypeMongo:
		if err := b.RequireDependency(b.mongoDatabase, "mongo database"); err != nil {
			return nil, err
		}
		return b.createMongoStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *KeeperBuilder) createMemoryStorage() (*memorystorage.Storage, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return memorystorage.New(memorystorage.WithTTL(b.cfg.TTL),
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	), nil
}

// createRedisStorage creates a Redis storage from configuration.
func (b *KeeperBuilder) createRedisStorage() (*redisstorage.Storage, error) {
	if b.cfg.Storage.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return redisstorage.New(b.redisClient,
		redisstorage.WithTTL(b.cfg.TTL),
		redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeysPrefix),
	), nil
}

// createMongoStorage creates a MongoDB storage from configuration.
func (b *KeeperBuilder) createMongoStorage() (*mongostorage.Storage, error) {
	if b.cfg.Storage.Mongo == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	return mongostorage.New(b.mongoDatabase,
		mongostorage.WithCollectionName(b.cfg.Storage.Mongo.Collection),
		mongostorage.WithTTL(b.cfg.TTL),
	)
}

// createNatsStorage creates a NATS storage from configuration.
func (b *KeeperBuilder) createNatsStorage() (*natsstorage.Storage, error) {
	if b.cfg.Storage.NATS == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := []natsstorage.Option{
		natsstorage.WithMaxAge(b.cfg.TTL),
		natsstorage.WithBucket(b.cfg.Storage.NATS.Bucket),
		natsstorage.WithReplicas(b.cfg.Storage.NATS.Replicas),
	}
	opts = coreslices.AppendIf(opts, b.cfg.Storage.NATS.MigrateBucketTTL, natsstorage.WithMigrateBucketTTL())
	opts = coreslices.AppendIf(opts, b.cfg.Storage.NATS.StrictBucketStorage, natsstorage.WithStrictBucketStorage())

	return natsstorage.New(b.jetstream, opts...)
}
