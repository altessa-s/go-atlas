// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/mongo"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	memorystorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
)

const (
	// DefaultBucket is the default NATS KeyValue bucket name for cursor storage.
	DefaultBucket = "cursor"
)

// CursorStorageBuilder assembles a [mongo.CursorStorage] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [CursorStorageBuilder.Build] time.
// The builder is not safe for concurrent use.
type CursorStorageBuilder struct {
	corefactory.Base
	cfg  *config.CacheStorageConfig
	errs []error
	ttl  time.Duration

	// Dependencies
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	scheduler   corescheduler.TaskRegistrar
}

// New creates a [CursorStorageBuilder] for the given cache storage config.
// Config can be nil — the error surfaces at [CursorStorageBuilder.Build] time.
func New(cfg *config.CacheStorageConfig) *CursorStorageBuilder {
	return &CursorStorageBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the cursor storage. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *CursorStorageBuilder) Build(ctx context.Context) (mongo.CursorStorage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Type {
	case config.CacheStorageTypeMemory:
		return b.createMemoryStorage(), nil //nolint:contextcheck // memory storage handles its own context
	case config.CacheStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case config.CacheStorageTypeNats:
		if err := b.RequireDependency(b.jetstream, "jetstream"); err != nil {
			return nil, err
		}
		return b.createNatsStorage(ctx)
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Type)
	}
}

// createMemoryStorage creates an in-memory cursor storage from configuration.
func (b *CursorStorageBuilder) createMemoryStorage() *memorystorage.Storage {
	opts := []memorystorage.Option{
		memorystorage.WithScheduler(b.scheduler),
	}
	if b.cfg.Memory != nil && b.cfg.Memory.CleanupSchedule != "" {
		opts = append(opts, memorystorage.WithCleanupSchedule(b.cfg.Memory.CleanupSchedule))
	}
	return memorystorage.New(b.ttl, opts...)
}

// createRedisStorage creates a Redis cursor storage from configuration.
func (b *CursorStorageBuilder) createRedisStorage() (*redisstorage.Storage, error) {
	if b.cfg.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := []redisstorage.Option{
		redisstorage.WithKeyPrefix(b.cfg.Redis.KeysPrefix),
		redisstorage.WithTtl(b.ttl),
	}

	return redisstorage.New(b.redisClient, opts...), nil
}

// createNatsStorage creates a NATS cursor storage from configuration.
// This method creates the KeyValue bucket with the specified TTL.
func (b *CursorStorageBuilder) createNatsStorage(ctx context.Context) (*natsstorage.Storage, error) {
	if b.cfg.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	bucket := cmp.Or(b.cfg.Nats.Bucket, DefaultBucket)
	replicas := cmp.Or(b.cfg.Nats.Replicas, 1)

	kv, err := b.jetstream.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:   bucket,
		TTL:      b.ttl,
		Storage:  jetstream.FileStorage,
		Replicas: replicas,
	})
	if err != nil {
		return nil, b.WrapError(err, "failed to create NATS KeyValue bucket")
	}

	return natsstorage.New(kv), nil
}
