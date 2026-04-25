// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	memorystorage "github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

// KeeperBuilder assembles an idempotency [idempotency.Keeper] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [KeeperBuilder.Build] time.
// The builder is not safe for concurrent use.
type KeeperBuilder struct {
	corefactory.Base
	cfg  *config.Idempotency
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	scheduler   corescheduler.TaskRegistrar
	collector   metrics.Collector
}

// New creates a [KeeperBuilder] for the given idempotency config.
// Config can be nil — the error surfaces at [KeeperBuilder.Build] time.
func New(cfg *config.Idempotency) *KeeperBuilder {
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
	return idempotency.New(storage, opts...)
}

// createStorage creates a storage backend based on configuration.
// The ttl parameter is applied to the storage backend for key expiration.
func (b *KeeperBuilder) createStorage() (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Storage.Type {
	case config.CacheStorageTypeMemory:
		return b.createMemoryStorage()
	case config.CacheStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case config.CacheStorageTypeNats:
		if err := b.RequireDependency(b.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return b.createNatsStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *KeeperBuilder) createMemoryStorage() (*memorystorage.Storage, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return memorystorage.New(memorystorage.WithTtl(b.cfg.TTL),
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
		redisstorage.WithTtl(b.cfg.TTL),
		redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeysPrefix),
	), nil
}

// createNatsStorage creates a NATS storage from configuration.
func (b *KeeperBuilder) createNatsStorage() (*natsstorage.Storage, error) {
	if b.cfg.Storage.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return natsstorage.New(b.jetstream,
		natsstorage.WithMaxAge(b.cfg.TTL),
		natsstorage.WithBucket(b.cfg.Storage.Nats.Bucket),
		natsstorage.WithReplicas(b.cfg.Storage.Nats.Replicas),
	)
}
