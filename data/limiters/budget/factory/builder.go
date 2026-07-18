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
	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	memorystorage "github.com/altessa-s/go-atlas/data/limiters/storages/memory"
	natsstorage "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
	redisstorage "github.com/altessa-s/go-atlas/data/limiters/storages/redis"
)

// BudgetLimiterBuilder assembles a [budget.Limiter] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [BudgetLimiterBuilder.Build] time.
// The builder is not safe for concurrent use.
type BudgetLimiterBuilder struct {
	corefactory.Base
	cfg  *config.BudgetLimiter
	errs []error

	// Dependencies
	redisClient redis.UniversalClient
	jetstream   jetstream.JetStream
	scheduler   corescheduler.TaskRegistrar
	collector   metrics.Collector
}

// New creates a [BudgetLimiterBuilder] for the given budget limiter config.
// Config can be nil — the error surfaces at [BudgetLimiterBuilder.Build] time.
func New(cfg *config.BudgetLimiter) *BudgetLimiterBuilder {
	return &BudgetLimiterBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the budget limiter. It creates the storage from config internally,
// then creates the limiter. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *BudgetLimiterBuilder) Build() (*budget.Limiter, error) {
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

	settings := &budget.Settings{
		Limit:  b.cfg.Limit,
		Period: b.cfg.Period,
	}

	opts := b.applyDefaults()

	return budget.New(settings, storage, opts...)
}

// applyDefaults returns factory defaults as limiter options.
func (b *BudgetLimiterBuilder) applyDefaults() []budget.Option {
	return []budget.Option{
		budget.WithCollector(b.collector),
	}
}

// createStorage creates a storage backend based on configuration.
func (b *BudgetLimiterBuilder) createStorage() (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, fmt.Errorf("storage configuration is required")
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
func (b *BudgetLimiterBuilder) createMemoryStorage() (*memorystorage.Provider, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, fmt.Errorf("memory storage configuration is required")
	}

	return memorystorage.New(
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	), nil
}

// createRedisStorage creates a Redis storage from configuration.
func (b *BudgetLimiterBuilder) createRedisStorage() (*redisstorage.Provider, error) {
	if b.cfg.Storage.Redis == nil {
		return nil, fmt.Errorf("redis storage configuration is required")
	}

	return redisstorage.New(b.redisClient, redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeysPrefix)), nil
}

// createNatsStorage creates a NATS storage from configuration.
func (b *BudgetLimiterBuilder) createNatsStorage() (*natsstorage.Provider, error) {
	if b.cfg.Storage.Nats == nil {
		return nil, fmt.Errorf("nats storage configuration is required")
	}

	return natsstorage.New(b.jetstream,
		natsstorage.WithBucket(b.cfg.Storage.Nats.Bucket),
		natsstorage.WithReplicas(b.cfg.Storage.Nats.Replicas),
	)
}
