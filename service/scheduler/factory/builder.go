// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/service/scheduler"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
	redisstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
)

// SchedulerBuilder assembles a [scheduler.Scheduler] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [SchedulerBuilder.Build] time.
// The builder is not safe for concurrent use.
type SchedulerBuilder struct {
	corefactory.Base
	cfg  *config.Scheduler
	errs []error

	// Dependencies
	leaderElector leadelect.LeaderElector
	collector     metrics.Collector
	mongoDb       *mongo.Database
	redisClient   redis.UniversalClient
}

// New creates a new [SchedulerBuilder] for the given scheduler config.
// Config can be nil -- the error surfaces at [SchedulerBuilder.Build] time.
func New(cfg *config.Scheduler) *SchedulerBuilder {
	return &SchedulerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the scheduler. Storage is created internally from cfg.Storage.
// Errors from fluent methods are accumulated and reported here via [errors.Join].
func (b *SchedulerBuilder) Build() (*scheduler.Scheduler, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := b.createStorageFromConfig()
	if err != nil {
		return nil, err
	}

	configOpts := []scheduler.Option{
		scheduler.WithTickInterval(b.cfg.TickInterval),
		scheduler.WithHistoryRetention(b.cfg.HistoryRetention),
		scheduler.WithMaxConcurrentTasks(b.cfg.MaxConcurrentTasks),
		scheduler.WithReservedHighPrioritySlots(b.cfg.ReservedHighPrioritySlots),
		scheduler.WithStaleTaskTimeout(b.cfg.StaleTaskTimeout),
		scheduler.WithCollector(b.collector),
	}

	opts := b.applyDefaults(configOpts)
	return scheduler.New(storage, opts...), nil
}

// applyDefaults prepends factory defaults to scheduler options.
func (b *SchedulerBuilder) applyDefaults(opts []scheduler.Option) []scheduler.Option {
	var defaults []scheduler.Option
	// Base.Logger() never returns nil
	defaults = append(defaults, scheduler.WithLogger(b.Logger()))
	if b.leaderElector != nil {
		defaults = append(defaults, scheduler.WithLeaderElector(b.leaderElector))
	}
	return append(defaults, opts...)
}

// createStorageFromConfig creates a [scheduler.Storage] backend based on the
// storage type specified in cfg.
func (b *SchedulerBuilder) createStorageFromConfig() (scheduler.Storage, error) {
	if b.cfg.Storage == nil {
		return b.createMemoryStorage()
	}

	switch b.cfg.Storage.Type {
	case config.SchedulerStorageTypeMemory, "":
		return b.createMemoryStorage()
	case config.SchedulerStorageTypeMongodb:
		if err := b.RequireDependency(b.mongoDb, "mongo database"); err != nil {
			return nil, err
		}
		return b.createMongoStorage()
	case config.SchedulerStorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates an in-memory storage backend.
// Error return is kept for interface consistency with other create*Storage methods.
func (b *SchedulerBuilder) createMemoryStorage() (*memorystorage.Storage, error) { //nolint:unparam
	if b.cfg.Storage == nil || b.cfg.Storage.Memory == nil {
		return memorystorage.New(config.DefaultSchedulerStorageMemoryConfig().MaxHistoryPerTask), nil
	}
	return memorystorage.New(b.cfg.Storage.Memory.MaxHistoryPerTask), nil
}

// createMongoStorage creates a MongoDB storage backend.
func (b *SchedulerBuilder) createMongoStorage() (*mongostorage.Storage, error) {
	if b.cfg.Storage.Mongodb == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]mongostorage.Option, 0, 2)
	opts = append(opts, mongostorage.WithTasksCollection(b.cfg.Storage.Mongodb.TasksCollection))
	opts = append(opts, mongostorage.WithHistoryCollection(b.cfg.Storage.Mongodb.HistoryCollection))

	return mongostorage.New(b.mongoDb, opts...), nil
}

// createRedisStorage creates a Redis storage backend.
func (b *SchedulerBuilder) createRedisStorage() (*redisstorage.Storage, error) {
	if b.cfg.Storage.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]redisstorage.Option, 0, 3)
	opts = append(opts, redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeyPrefix))
	opts = append(opts, redisstorage.WithHistoryTTL(b.cfg.Storage.Redis.HistoryTTL))
	opts = append(opts, redisstorage.WithMaxHistoryPerTask(b.cfg.Storage.Redis.MaxHistoryPerTask))

	return redisstorage.New(b.redisClient, opts...), nil
}
