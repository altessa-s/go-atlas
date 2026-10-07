// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/service/scheduler"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	chstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/clickhouse"
	memorystorage "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/service/scheduler/storages/mongo"
	redisstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
	sqlstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
)

// SchedulerBuilder assembles a [scheduler.Scheduler] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [SchedulerBuilder.Build] time.
// The builder is not safe for concurrent use.
type SchedulerBuilder struct {
	corefactory.Base
	cfg  *schedulerconfig.Config
	errs []error

	// Dependencies
	leaderElector  scheduler.LeaderElector
	collector      metrics.Collector
	readinessProbe func() bool
	mongoDb        *mongo.Database
	redisClient    redis.UniversalClient
	sqlDB          *sql.DB
	clickhouseConn chstorage.Conn
}

// New creates a new [SchedulerBuilder] for the given scheduler config.
// Config can be nil -- the error surfaces at [SchedulerBuilder.Build] time.
func New(cfg *schedulerconfig.Config) *SchedulerBuilder {
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

	var history scheduler.HistoryStorage
	if b.cfg.HistoryStorage != nil {
		if history, err = b.createHistoryStorageFromConfig(); err != nil {
			return nil, err
		}
	}

	concurrencyOpts, err := b.concurrencyOptions()
	if err != nil {
		return nil, err
	}

	configOpts := make([]scheduler.Option, 0, 7+len(concurrencyOpts)) //nolint:mnd
	configOpts = append(configOpts,
		scheduler.WithTickInterval(b.cfg.TickInterval),
		scheduler.WithHistoryRetention(b.cfg.HistoryRetention),
		scheduler.WithReservedHighPrioritySlots(b.cfg.Concurrency.ReservedHighPrioritySlots),
		scheduler.WithStaleTaskTimeout(b.cfg.StaleTaskTimeout),
		scheduler.WithInstanceID(b.cfg.InstanceID),
		scheduler.WithCollector(b.collector),
	)
	configOpts = slices.AppendIf(configOpts, history != nil, scheduler.WithHistoryStorage(history))
	configOpts = append(configOpts, concurrencyOpts...)

	opts := b.applyDefaults(configOpts)
	return scheduler.New(storage, opts...), nil
}

// applyDefaults prepends factory defaults to scheduler options.
func (b *SchedulerBuilder) applyDefaults(opts []scheduler.Option) []scheduler.Option {
	var defaults []scheduler.Option
	// Base.Logger() never returns nil
	defaults = append(defaults, scheduler.WithLogger(b.Logger()))
	defaults = slices.AppendIf(defaults, nilcheck.IsNotNil(b.leaderElector), scheduler.WithLeaderElector(b.leaderElector))
	defaults = slices.AppendIf(defaults, b.readinessProbe != nil, scheduler.WithReadinessProbe(b.readinessProbe))
	return append(defaults, opts...)
}

// createStorageFromConfig creates a [scheduler.Storage] backend based on the
// storage type specified in cfg.
func (b *SchedulerBuilder) createStorageFromConfig() (scheduler.Storage, error) {
	if b.cfg.Storage == nil {
		return b.createMemoryStorage()
	}

	switch b.cfg.Storage.Type {
	case schedulerconfig.StorageTypeMemory, "":
		return b.createMemoryStorage()
	case schedulerconfig.StorageTypeMongo:
		if err := b.RequireDependency(b.mongoDb, "mongo database"); err != nil {
			return nil, err
		}
		return b.createMongoStorage()
	case schedulerconfig.StorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "redis client"); err != nil {
			return nil, err
		}
		return b.createRedisStorage()
	case schedulerconfig.StorageTypeSQL:
		if err := b.RequireDependency(b.sqlDB, "sql database"); err != nil {
			return nil, err
		}
		return b.createSQLStorage()
	default:
		return nil, b.Errorf("unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates an in-memory storage backend.
func (b *SchedulerBuilder) createMemoryStorage() (*memorystorage.Storage, error) {
	if b.cfg.Storage == nil || b.cfg.Storage.Memory == nil {
		return memorystorage.New(schedulerconfig.DefaultStorageMemoryConfig().MaxHistoryPerTask)
	}
	return memorystorage.New(b.cfg.Storage.Memory.MaxHistoryPerTask)
}

// createMongoStorage creates a MongoDB storage backend. With ensureIndexes set
// it creates the indexes through the storage's EnsureIndexes; otherwise it
// performs no I/O and the indexes are expected to exist.
func (b *SchedulerBuilder) createMongoStorage() (*mongostorage.Storage, error) {
	if b.cfg.Storage.Mongo == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]mongostorage.Option, 0, 2)
	opts = append(opts, mongostorage.WithTasksCollection(b.cfg.Storage.Mongo.TasksCollection))
	opts = append(opts, mongostorage.WithHistoryCollection(b.cfg.Storage.Mongo.HistoryCollection))

	storage := mongostorage.New(b.mongoDb, opts...)
	if !b.cfg.Storage.Mongo.EnsureIndexes {
		return storage, nil
	}
	if err := b.ensure("ensure scheduler indexes", storage.EnsureIndexes); err != nil {
		return nil, err
	}
	return storage, nil
}

// createRedisStorage creates a Redis storage backend. With ensureIndexes set it
// creates the RediSearch indexes through the storage's EnsureIndexes;
// otherwise it performs no I/O and the indexes are expected to exist.
func (b *SchedulerBuilder) createRedisStorage() (*redisstorage.Storage, error) {
	if b.cfg.Storage.Redis == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]redisstorage.Option, 0, 3)
	opts = append(opts, redisstorage.WithKeyPrefix(b.cfg.Storage.Redis.KeyPrefix))
	opts = append(opts, redisstorage.WithHistoryTTL(b.cfg.Storage.Redis.HistoryTTL))
	opts = append(opts, redisstorage.WithMaxHistoryPerTask(b.cfg.Storage.Redis.MaxHistoryPerTask))

	storage := redisstorage.New(b.redisClient, opts...)
	if !b.cfg.Storage.Redis.EnsureIndexes {
		return storage, nil
	}
	if err := b.ensure("ensure scheduler indexes", storage.EnsureIndexes); err != nil {
		return nil, err
	}
	return storage, nil
}

// ensureTimeout bounds the schema or index creation the factory runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureTimeout = 30 * time.Second

// ensure runs a storage's idempotent provisioning step under [ensureTimeout].
func (b *SchedulerBuilder) ensure(op string, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), ensureTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		return b.WrapError(err, op)
	}
	return nil
}

// createSQLStorage creates a SQL storage backend. With ensureSchema set it
// creates or upgrades the schema through the storage's EnsureSchema; otherwise,
// like the MongoDB backend, it performs no I/O and the schema is expected to
// exist — created by EnsureSchema on a sqldb storage with the same handle,
// dialect and tables, or applied through migrations.
func (b *SchedulerBuilder) createSQLStorage() (*sqlstorage.Storage, error) {
	if b.cfg.Storage.SQL == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := sqlstorage.New(b.sqlDB, sqlstorage.Dialect(b.cfg.Storage.SQL.Dialect),
		sqlstorage.WithTasksTable(b.cfg.Storage.SQL.TasksTable),
		sqlstorage.WithHistoryTable(b.cfg.Storage.SQL.HistoryTable),
	)
	if err != nil || !b.cfg.Storage.SQL.EnsureSchema {
		return storage, err
	}
	if err := b.ensure("ensure scheduler schema", storage.EnsureSchema); err != nil {
		return nil, err
	}
	return storage, nil
}

// createHistoryStorageFromConfig creates the history storage set by a non-nil
// cfg.HistoryStorage.
func (b *SchedulerBuilder) createHistoryStorageFromConfig() (scheduler.HistoryStorage, error) {
	switch b.cfg.HistoryStorage.Type {
	case schedulerconfig.HistoryStorageTypeClickHouse:
		if err := b.RequireDependency(b.clickhouseConn, "clickhouse connection"); err != nil {
			return nil, err
		}
		return b.createClickHouseHistoryStorage()
	default:
		return nil, b.Errorf("unsupported history storage type: %s", b.cfg.HistoryStorage.Type)
	}
}

// createClickHouseHistoryStorage creates a ClickHouse history storage. A zero
// TTL takes the scheduler's history retention. With ensureSchema set it creates
// the table through the storage's EnsureSchema; otherwise it performs no I/O
// and the table is expected to exist.
func (b *SchedulerBuilder) createClickHouseHistoryStorage() (*chstorage.Storage, error) {
	cfg := b.cfg.HistoryStorage.ClickHouse
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	storage, err := chstorage.New(b.clickhouseConn,
		chstorage.WithTableName(cfg.TableName),
		chstorage.WithEngine(cfg.Engine),
		chstorage.WithCluster(cfg.Cluster),
		chstorage.WithTTL(cmp.Or(cfg.TTL, b.cfg.HistoryRetention)),
	)
	if err != nil {
		return nil, b.WrapError(err, "create scheduler history storage")
	}
	if !cfg.EnsureSchema {
		return storage, nil
	}
	if err := b.ensure("ensure scheduler history schema", storage.EnsureSchema); err != nil {
		return nil, err
	}
	return storage, nil
}

// concurrencyOptions maps the concurrency config strategy to scheduler options.
// The static strategy uses the efficient semaphore-based [scheduler.WithMaxConcurrentTasks];
// all other strategies use [scheduler.WithConcurrencyLimitFunc] for dynamic evaluation.
func (b *SchedulerBuilder) concurrencyOptions() ([]scheduler.Option, error) {
	if b.cfg.Concurrency.Strategy == schedulerconfig.ConcurrencyStatic || b.cfg.Concurrency.Strategy == "" {
		return []scheduler.Option{
			scheduler.WithMaxConcurrentTasks(b.cfg.Concurrency.MaxTasks),
		}, nil
	}

	limitFunc, err := b.cfg.Concurrency.BuildLimitFunc()
	if err != nil {
		return nil, err
	}

	return []scheduler.Option{
		scheduler.WithConcurrencyLimitFunc(limitFunc),
	}, nil
}
