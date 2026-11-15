// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/altessa-s/go-horai/scheduler"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	horai "github.com/altessa-s/go-horai"
	memorystorage "github.com/altessa-s/go-horai/storage/memory"
	sqlstorage "github.com/altessa-s/go-horai/storage/sqldb"
)

// SchedulerBuilder assembles a go-horai [scheduler.Scheduler] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [SchedulerBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds the memory and sqldb task storages. The MongoDB and Redis
// ones register their [StorageFunc] with [SchedulerBuilder.UseStorage] from
// factory/ext/horai/mongo and
// factory/ext/horai/redis.
//
// The builder builds no history storage of its own: the ClickHouse one
// registers its [HistoryStorageBuilder] with [SchedulerBuilder.UseHistoryStorage]
// from factory/ext/horai/clickhouse.
type SchedulerBuilder struct {
	corefactory.Base
	cfg  *schedulerconfig.Config
	errs []error

	leaderElector  horai.LeaderElector
	collector      metrics.Meter
	readinessProbe func() bool
	sqlDB          *sql.DB
	storages       map[schedulerconfig.StorageType]StorageFunc
	histories      map[schedulerconfig.HistoryStorageType]HistoryStorageBuilder
}

// StorageFunc creates the task storage of one storage type from the scheduler
// configuration.
type StorageFunc func(ctx context.Context, cfg *schedulerconfig.Config) (horai.Store, error)

// HistoryStorageBuilder creates the history storage of one history storage
// type from the scheduler configuration.
type HistoryStorageBuilder func(ctx context.Context, cfg *schedulerconfig.Config) (horai.HistoryStore, error)

// New creates a [SchedulerBuilder] for the given scheduler config.
// Config can be nil — the error surfaces at [SchedulerBuilder.Build] time.
func New(cfg *schedulerconfig.Config) *SchedulerBuilder {
	return &SchedulerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the scheduler. Errors from fluent methods are accumulated
// and reported here via [errors.Join]. The caller starts and stops it.
func (b *SchedulerBuilder) Build(ctx context.Context) (*scheduler.Scheduler, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := b.createStorageFromConfig(ctx)
	if err != nil {
		return nil, err
	}

	var history horai.HistoryStore
	if b.cfg.HistoryStorage != nil {
		if history, err = b.createHistoryStorageFromConfig(ctx); err != nil {
			return nil, err
		}
	}

	concurrencyOpts, err := b.concurrencyOptions()
	if err != nil {
		return nil, err
	}

	opts := []scheduler.Option{
		scheduler.WithLogger(b.Logger()),
		scheduler.WithTickInterval(b.cfg.TickInterval),
		scheduler.WithHistoryRetention(b.cfg.HistoryRetention),
		scheduler.WithReservedHighPrioritySlots(b.cfg.Concurrency.ReservedHighPrioritySlots),
		scheduler.WithStaleTaskTimeout(b.cfg.StaleTaskTimeout),
		scheduler.WithInstanceID(b.cfg.InstanceID),
		scheduler.WithMetrics(newMetrics(b.collector)),
	}
	opts = slices.AppendIf(opts, nilcheck.IsNotNil(b.leaderElector), scheduler.WithLeaderElector(b.leaderElector))
	opts = slices.AppendIf(opts, b.readinessProbe != nil, scheduler.WithReadiness(horai.ReadinessFunc(b.readinessProbe)))
	opts = slices.AppendIf(opts, history != nil, scheduler.WithHistoryStore(history))
	opts = append(opts, concurrencyOpts...)

	s, err := scheduler.New(storage, opts...)
	if err != nil {
		return nil, b.WrapError(err, "create scheduler")
	}
	return s, nil
}

// createStorageFromConfig creates the task storage selected by the
// configuration: memory (the default) or sqldb, or the one registered for
// its type.
func (b *SchedulerBuilder) createStorageFromConfig(ctx context.Context) (horai.Store, error) {
	if b.cfg.Storage == nil {
		return b.createMemoryStorage()
	}

	switch b.cfg.Storage.Type {
	case schedulerconfig.StorageTypeMemory, "":
		return b.createMemoryStorage()
	case schedulerconfig.StorageTypeSQL:
		if err := b.RequireDependency(b.sqlDB, "sql database"); err != nil {
			return nil, err
		}
		return b.createSQLStorage(ctx)
	case schedulerconfig.StorageTypeMongo:
		build, ok := b.storages[b.cfg.Storage.Type]
		if !ok {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "scheduler storage %q is not registered: add factory/ext/horai/mongo "+
				"with UseStorage(%q, mongofactory.Builder(db))", b.cfg.Storage.Type, b.cfg.Storage.Type)
		}
		return build(ctx, b.cfg)
	case schedulerconfig.StorageTypeRedis:
		build, ok := b.storages[b.cfg.Storage.Type]
		if !ok {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "scheduler storage %q is not registered: add factory/ext/horai/redis "+
				"with UseStorage(%q, redisfactory.Builder(client))", b.cfg.Storage.Type, b.cfg.Storage.Type)
		}
		return build(ctx, b.cfg)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createMemoryStorage creates the in-process task storage.
func (b *SchedulerBuilder) createMemoryStorage() (*memorystorage.Store, error) {
	maxHistory := schedulerconfig.DefaultStorageMemoryConfig().MaxHistoryPerTask
	if b.cfg.Storage != nil && b.cfg.Storage.Memory != nil {
		maxHistory = b.cfg.Storage.Memory.MaxHistoryPerTask
	}
	return memorystorage.New(memorystorage.WithMaxHistoryPerTask(maxHistory))
}

// ensureTimeout bounds the schema or index creation of a storage at startup.
const ensureTimeout = 30 * time.Second

// ensure runs a schema or index creation step within [ensureTimeout].
func (b *SchedulerBuilder) ensure(ctx context.Context, op string, fn func(context.Context) error) error {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, ensureTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		return b.WrapError(err, op)
	}
	return nil
}

// createSQLStorage creates the database/sql task storage, ensuring its schema
// when configured.
func (b *SchedulerBuilder) createSQLStorage(ctx context.Context) (*sqlstorage.Store, error) {
	if b.cfg.Storage.SQL == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := sqlstorage.New(b.sqlDB, sqlstorage.Dialect(b.cfg.Storage.SQL.Dialect),
		sqlstorage.WithTasksTable(b.cfg.Storage.SQL.TasksTable),
		sqlstorage.WithHistoryTable(b.cfg.Storage.SQL.HistoryTable),
	)
	if err != nil || !b.cfg.Storage.SQL.EnsureSchema {
		return storage, err
	}
	if err := b.ensure(ctx, "ensure scheduler schema", storage.EnsureSchema); err != nil {
		return nil, err
	}
	return storage, nil
}

// createHistoryStorageFromConfig creates the history storage registered for
// the configured history storage type.
func (b *SchedulerBuilder) createHistoryStorageFromConfig(ctx context.Context) (horai.HistoryStore, error) {
	typ := b.cfg.HistoryStorage.Type
	build, ok := b.histories[typ]
	if ok {
		return build(ctx, b.cfg)
	}
	if typ == schedulerconfig.HistoryStorageTypeClickHouse {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "history storage %q is not registered: add factory/ext/horai/clickhouse "+
			"with UseHistoryStorage(%q, chfactory.Builder(conn))", typ, typ)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported history storage type: %s", typ)
}

// concurrencyOptions maps the concurrency strategy: a static limit, or a
// limit function evaluated on every tick.
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
		scheduler.WithConcurrencyLimit(limitFunc),
	}, nil
}
