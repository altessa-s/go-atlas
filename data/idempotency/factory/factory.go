// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

// Factory creates idempotency storage and keepers with configured backends.
type Factory struct {
	corefactory.Base
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	opts        *options
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		redisClient: cfg.redisClient,
		jetstream:   cfg.jetstream,
		opts:        cfg,
	}
}

// CreateMemoryStorageFromConfig creates an in-memory storage from configuration.
func (f *Factory) CreateMemoryStorageFromConfig(
	cfg *config.StorageMemoryConfig,
	ttl time.Duration,
) (*memorystorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return memorystorage.New(memorystorage.WithTtl(ttl),
		memorystorage.WithCleanupSchedule(cfg.CleanupSchedule),
		memorystorage.WithScheduler(f.opts.scheduler),
	), nil
}

// CreateRedisStorageFromConfig creates a Redis storage from configuration.
func (f *Factory) CreateRedisStorageFromConfig(
	cfg *config.StorageRedisConfig,
	ttl time.Duration,
) (*redisstorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return redisstorage.New(f.redisClient,
		redisstorage.WithTtl(ttl),
		redisstorage.WithKeyPrefix(cfg.KeysPrefix),
	), nil
}

// CreateNatsStorageFromConfig creates a NATS storage from configuration.
func (f *Factory) CreateNatsStorageFromConfig(
	cfg *config.StorageNATSConfig,
	ttl time.Duration,
) (*natsstorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	return natsstorage.New(f.jetstream,
		natsstorage.WithMaxAge(ttl),
		natsstorage.WithBucket(cfg.Bucket),
		natsstorage.WithReplicas(cfg.Replicas),
	)
}

// CreateStorageFromConfig creates a storage backend based on configuration.
// The ttl parameter is applied to the storage backend for key expiration.
func (f *Factory) CreateStorageFromConfig(
	cfg *config.CacheStorageConfig,
	ttl time.Duration,
) (storages.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Type {
	case config.CacheStorageTypeMemory:
		return f.CreateMemoryStorageFromConfig(cfg.Memory, ttl)
	case config.CacheStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return f.CreateRedisStorageFromConfig(cfg.Redis, ttl)
	case config.CacheStorageTypeNats:
		if err := f.RequireDependency(f.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return f.CreateNatsStorageFromConfig(cfg.Nats, ttl)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// createKeeper creates a Keeper with the given storage.
func (f *Factory) createKeeper(storage storages.Storage) *idempotency.Keeper {
	opts := []idempotency.Option{
		idempotency.WithLogger(f.Logger()),
	}
	return idempotency.New(storage, opts...)
}

// CreateKeeperFromConfig creates a Keeper from configuration.
func (f *Factory) CreateKeeperFromConfig(cfg *config.Idempotency) (*idempotency.Keeper, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := f.CreateStorageFromConfig(cfg.Storage, cfg.TTL)
	if err != nil {
		return nil, err
	}

	return f.createKeeper(storage), nil
}
