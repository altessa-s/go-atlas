// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/service/scheduler"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	memorystorage "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
	redisstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
)

// Factory creates [scheduler.Scheduler] instances and their [scheduler.Storage]
// backends from configuration objects or explicit options. It embeds
// [corefactory.Base] for shared logger, dependency-validation, and error-formatting
// helpers.
//
// A Factory carries optional infrastructure references (MongoDB database, Redis
// client, leader elector) that are injected at construction time via [Option]
// functions and forwarded to the schedulers and storage backends it creates.
// Not every reference is required: only the ones needed by the storage type
// requested at creation time must be non-nil.
//
// Factory is safe for concurrent use once constructed; all creation methods are
// stateless readers of the struct fields set during [New].
type Factory struct {
	corefactory.Base
	leaderElector leadelect.LeaderElector
	mongoDb       *mongo.Database
	redisClient   redis.UniversalClient
}

// New creates a new [Factory] configured with the supplied [Option] functions.
//
// At minimum a logger is always present: if none is provided via [WithLogger],
// a no-op discard logger is used. Infrastructure clients ([WithMongoDb],
// [WithRedisClient]) and the leader elector ([WithLeaderElector]) are optional
// and only required when the corresponding storage or scheduler features are
// used later.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:          corefactory.NewBase(cfg.logger),
		leaderElector: cfg.leaderElector,
		mongoDb:       cfg.mongoDb,
		redisClient:   cfg.redisClient,
	}
}

// createScheduler creates a Scheduler with the given storage backend.
// Factory defaults (logger, leader elector) are prepended to the provided options.
func (f *Factory) createScheduler(storage scheduler.Storage, opts ...scheduler.Option) *scheduler.Scheduler {
	opts = f.applyDefaults(opts)
	return scheduler.New(storage, opts...)
}

// createMongoStorage creates a MongoDB storage backend.
func (f *Factory) createMongoStorage(opts ...mongostorage.Option) *mongostorage.Storage {
	return mongostorage.New(f.mongoDb, opts...)
}

// createRedisStorage creates a Redis storage backend.
func (f *Factory) createRedisStorage(opts ...redisstorage.Option) *redisstorage.Storage {
	return redisstorage.New(f.redisClient, opts...)
}

// applyDefaults prepends factory defaults to scheduler options.
func (f *Factory) applyDefaults(opts []scheduler.Option) []scheduler.Option {
	var defaults []scheduler.Option
	// Base.Logger() never returns nil
	defaults = append(defaults, scheduler.WithLogger(f.Logger()))
	if f.leaderElector != nil {
		defaults = append(defaults, scheduler.WithLeaderElector(f.leaderElector))
	}
	return append(defaults, opts...)
}

// CreateSchedulerFromConfig creates a [scheduler.Scheduler] by translating the
// fields of cfg into [scheduler.Option] values (tick interval, history retention,
// concurrency limits, stale-task timeout) and merging them with any additional
// opts provided by the caller. Caller-supplied opts take precedence because they
// are appended after the configuration-derived options.
//
// The Factory's logger and leader elector (if set) are automatically prepended
// as defaults, so callers do not need to supply them.
//
// Returns an error if cfg is nil.
func (f *Factory) CreateSchedulerFromConfig(
	cfg *config.Scheduler,
	storage scheduler.Storage,
	opts ...scheduler.Option,
) (*scheduler.Scheduler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	configOpts := []scheduler.Option{
		scheduler.WithTickInterval(cfg.TickInterval),
		scheduler.WithHistoryRetention(cfg.HistoryRetention),
		scheduler.WithMaxConcurrentTasks(cfg.MaxConcurrentTasks),
		scheduler.WithReservedHighPrioritySlots(cfg.ReservedHighPrioritySlots),
		scheduler.WithStaleTaskTimeout(cfg.StaleTaskTimeout),
	}

	opts = append(configOpts, opts...)
	return f.createScheduler(storage, opts...), nil
}

// CreateStorageFromConfig creates a [scheduler.Storage] backend based on the
// storage type specified in cfg. Supported types are:
//
//   - [config.SchedulerStorageTypeMemory] (or empty string) -- delegates to
//     [Factory.CreateMemoryStorageFromConfig]
//   - [config.SchedulerStorageTypeMongodb] -- delegates to
//     [Factory.CreateMongoStorageFromConfig]; requires a non-nil MongoDB database
//     to have been supplied via [WithMongoDb]
//   - [config.SchedulerStorageTypeRedis] -- delegates to
//     [Factory.CreateRedisStorageFromConfig]; requires a non-nil Redis client
//     to have been supplied via [WithRedisClient]
//
// If cfg is nil, in-memory storage with default settings is returned.
//
// Returns an error if the requested storage type requires an infrastructure
// client that was not provided to the Factory, or if the type is unrecognized.
func (f *Factory) CreateStorageFromConfig(cfg *config.SchedulerStorageConfig) (scheduler.Storage, error) {
	if cfg == nil {
		return f.CreateMemoryStorageFromConfig(nil)
	}

	switch cfg.Type {
	case config.SchedulerStorageTypeMemory, "":
		return f.CreateMemoryStorageFromConfig(cfg.Memory)
	case config.SchedulerStorageTypeMongodb:
		if err := f.RequireDependency(f.mongoDb, "mongo database"); err != nil {
			return nil, err
		}
		return f.CreateMongoStorageFromConfig(cfg.Mongodb)
	case config.SchedulerStorageTypeRedis:
		if err := f.RequireDependency(f.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return f.CreateRedisStorageFromConfig(cfg.Redis)
	default:
		return nil, f.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

// CreateMemoryStorageFromConfig creates an in-memory [scheduler.Storage]
// backend. If cfg is nil, [config.DefaultSchedulerStorageMemoryConfig] supplies
// the defaults (including the maximum history entries per task).
//
// The returned storage is suitable for development, testing, or single-instance
// deployments where persistence across restarts is not required.
func (f *Factory) CreateMemoryStorageFromConfig(cfg *config.SchedulerStorageMemoryConfig) (*memorystorage.Storage, error) {
	if cfg == nil {
		return memorystorage.New(config.DefaultSchedulerStorageMemoryConfig().MaxHistoryPerTask), nil
	}
	return memorystorage.New(cfg.MaxHistoryPerTask), nil
}

// CreateMongoStorageFromConfig creates a MongoDB-backed [scheduler.Storage]
// using the collection names specified in cfg. The underlying MongoDB database
// must have been provided to the Factory via [WithMongoDb].
//
// Returns an error if cfg is nil.
func (f *Factory) CreateMongoStorageFromConfig(cfg *config.SchedulerStorageMongoConfig) (*mongostorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]mongostorage.Option, 0, 2)
	opts = append(opts, mongostorage.WithTasksCollection(cfg.TasksCollection))
	opts = append(opts, mongostorage.WithHistoryCollection(cfg.HistoryCollection))

	return f.createMongoStorage(opts...), nil
}

// CreateRedisStorageFromConfig creates a Redis-backed [scheduler.Storage]
// using the key prefix, history TTL, and per-task history limit from cfg.
// The underlying Redis client must have been provided to the Factory via
// [WithRedisClient].
//
// Returns an error if cfg is nil.
func (f *Factory) CreateRedisStorageFromConfig(cfg *config.SchedulerStorageRedisConfig) (*redisstorage.Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]redisstorage.Option, 0, 3)
	opts = append(opts, redisstorage.WithKeyPrefix(cfg.KeyPrefix))
	opts = append(opts, redisstorage.WithHistoryTTL(cfg.HistoryTTL))
	opts = append(opts, redisstorage.WithMaxHistoryPerTask(cfg.MaxHistoryPerTask))

	return f.createRedisStorage(opts...), nil
}
