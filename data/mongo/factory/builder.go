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

	"github.com/altessa-s/go-atlas/data/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	natskvlease "github.com/altessa-s/go-atlas/data/internal/natskvlease"
	memorystorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
)

const (
	// DefaultBucket is the default NATS KeyValue bucket name for cursor storage.
	DefaultBucket = "cursor"
)

// ErrBucketTTLMismatch is returned by Build for NATS storage when the bucket
// already exists with a key TTL other than the builder's and migrateBucketTTL
// is not set. The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by Build for NATS storage when the
// bucket already exists with another storage type and strictBucketStorage is
// set. The bucket is left untouched.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// CursorStorageBuilder assembles a [mongo.CursorStorage] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [CursorStorageBuilder.Build] time.
// The builder is not safe for concurrent use.
type CursorStorageBuilder struct {
	corefactory.Base
	cfg  *storageconfig.CacheStorageConfig
	errs []error
	ttl  time.Duration

	// Dependencies
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	scheduler   corescheduler.TaskRegistrar
}

// New creates a [CursorStorageBuilder] for the given cache storage config.
// Config can be nil — the error surfaces at [CursorStorageBuilder.Build] time.
func New(cfg *storageconfig.CacheStorageConfig) *CursorStorageBuilder {
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
	case storageconfig.CacheStorageTypeMemory:
		return b.createMemoryStorage(), nil //nolint:contextcheck // memory storage handles its own context
	case storageconfig.CacheStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case storageconfig.CacheStorageTypeNats:
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
	opts = coreslices.AppendIfFunc(opts, b.cfg.Memory != nil && b.cfg.Memory.CleanupSchedule != "", func() []memorystorage.Option {
		return []memorystorage.Option{memorystorage.WithCleanupSchedule(b.cfg.Memory.CleanupSchedule)}
	})
	return memorystorage.New(b.ttl, opts...)
}

// createRedisStorage creates a Redis cursor storage from configuration.
func (b *CursorStorageBuilder) createRedisStorage() (*redisstorage.Storage, error) {
	if b.cfg.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := []redisstorage.Option{
		redisstorage.WithKeyPrefix(b.cfg.Redis.KeysPrefix),
		redisstorage.WithTTL(b.ttl),
	}

	return redisstorage.New(b.redisClient, opts...), nil
}

// createNatsStorage creates a NATS cursor storage from configuration.
//
// The bucket is created with the builder's TTL (zero: cursors never expire) or,
// when it exists, checked against it like every other NATS KV backend: a
// different key TTL fails with [natskvlease.ErrBucketTTLMismatch] unless
// migrateBucketTTL is set, and a different storage type is used as is with a
// warning unless strictBucketStorage is set.
func (b *CursorStorageBuilder) createNatsStorage(ctx context.Context) (*natsstorage.Storage, error) {
	if b.cfg.Nats == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	kv, err := natskvlease.NewKVHelper(b.jetstream, b.Logger()).GetOrCreateBucket(ctx, natskvlease.BucketConfig{
		Bucket:        cmp.Or(b.cfg.Nats.Bucket, DefaultBucket),
		TTL:           b.ttl,
		NoTTL:         b.ttl == 0,
		Storage:       jetstream.FileStorage,
		Replicas:      cmp.Or(b.cfg.Nats.Replicas, 1),
		MigrateTTL:    b.cfg.Nats.MigrateBucketTTL,
		StrictStorage: b.cfg.Nats.StrictBucketStorage,
	})
	if err != nil {
		return nil, b.WrapError(err, "failed to create NATS KeyValue bucket")
	}

	return natsstorage.New(kv), nil
}
