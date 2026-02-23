// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/mongo"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
)

const (
	// DefaultBucket is the default NATS KeyValue bucket name for cursor storage.
	DefaultBucket = "cursor"
)

// Factory creates cursor storages with configured storage backends.
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

// createCursorMemoryStorageFromConfig creates an in-memory cursor storage from configuration.
func (f *Factory) createCursorMemoryStorageFromConfig(cfg *config.StorageMemoryConfig, ttl time.Duration) *memorystorage.Storage {
	opts := []memorystorage.Option{
		memorystorage.WithScheduler(f.opts.scheduler),
	}
	if cfg != nil && cfg.CleanupSchedule != "" {
		opts = append(opts, memorystorage.WithCleanupSchedule(cfg.CleanupSchedule))
	}
	return memorystorage.New(ttl, opts...)
}

// CreateCursorRedisStorageFromConfig creates a Redis cursor storage from configuration.
func (f *Factory) CreateCursorRedisStorageFromConfig(
	cfg *config.StorageRedisConfig,
	ttl time.Duration,
) (*redisstorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := []redisstorage.Option{
		redisstorage.WithKeyPrefix(cfg.KeysPrefix),
		redisstorage.WithTtl(ttl),
	}

	return redisstorage.New(f.redisClient, opts...), nil
}

// CreateCursorNatsStorageFromConfig creates a NATS cursor storage from configuration.
// This method creates the KeyValue bucket with the specified TTL.
func (f *Factory) CreateCursorNatsStorageFromConfig(
	ctx context.Context,
	cfg *config.StorageNATSConfig,
	ttl time.Duration,
) (*natsstorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	bucket := cmp.Or(cfg.Bucket, DefaultBucket)
	replicas := cmp.Or(cfg.Replicas, 1)

	kv, err := f.jetstream.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   bucket,
		TTL:      ttl,
		Storage:  jetstream.FileStorage,
		Replicas: replicas,
	})
	if err != nil {
		return nil, f.WrapError(err, "failed to create NATS KeyValue bucket")
	}

	return natsstorage.New(kv), nil
}

// CreateCursorStorageFromConfig creates a cursor storage based on configuration.
func (f *Factory) CreateCursorStorageFromConfig(
	ctx context.Context,
	cfg *config.CacheStorageConfig,
	ttl time.Duration,
) (mongo.CursorStorage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Type {
	case config.CacheStorageTypeMemory:
		return f.createCursorMemoryStorageFromConfig(cfg.Memory, ttl), nil //nolint:contextcheck // memory storage handles its own context
	case config.CacheStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return f.CreateCursorRedisStorageFromConfig(cfg.Redis, ttl)
	case config.CacheStorageTypeNats:
		if err := f.RequireDependency(f.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return f.CreateCursorNatsStorageFromConfig(ctx, cfg.Nats, ttl)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
