// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// StorageFunc creates the outbox storage from the outbox configuration. ctx
// bounds the storage's startup I/O (index or schema creation); logger is the
// builder's logger.
type StorageFunc func(ctx context.Context, cfg *outboxconfig.Config, logger *slog.Logger) (outbox.Storage, error)

// OutboxBuilder assembles an [outbox.Outbox] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at build time.
// The builder is not safe for concurrent use.
//
// The storage is selected by outboxconfig.Config.Storage.Type. The builder
// builds the sqldb store itself over the handle given to
// [OutboxBuilder.UseSQLDB]. The MongoDB one registers its [StorageFunc] with
// [OutboxBuilder.UseStorage] from factory/reliability/outbox/storages/mongo; a
// configured type without one fails at Build naming the package to add.
type OutboxBuilder struct {
	corefactory.Base
	cfg  *outboxconfig.Config
	errs []error

	// Dependencies
	scheduler     schedtask.Registrar
	outboxOpts    []outbox.Option
	storages      map[outboxconfig.StorageType]StorageFunc
	sqlDB         *sql.DB
	resolveTx     func(context.Context) (*sql.Tx, bool)
	sqlListener   outboxsql.Listener
	shutdownHooks *coreruntime.HookGroup
}

// New creates a new [OutboxBuilder] for the given outbox config.
// Config can be nil -- the error surfaces at build time.
func New(cfg *outboxconfig.Config) *OutboxBuilder {
	return &OutboxBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates the outbox over the configured storage and registers its
// tasks with the scheduler. ctx bounds the storage's startup I/O and the task
// registration. Returns (nil, nil) when the outbox is disabled.
//
// The sqldb storage needs the handle from [OutboxBuilder.UseSQLDB]; with
// ensureSchema set, the events table is created if it does not exist,
// otherwise it must already exist. Save must run on the business transaction:
// pass outboxsql.WithTx(ctx, tx). The mongo storage needs the [StorageFunc]
// registered with [OutboxBuilder.UseStorage].
//
// With watch set to changeStream or notify, a successful build starts
// [outbox.Outbox.StartWatch]. The caller can stop and join it with
// [outbox.Outbox.StopWatch]. The shutdown hook group (the process registry unless
// [OutboxBuilder.UseShutdownHooks] says otherwise) also stops it. The notify mode makes the
// sqldb store signal PostgreSQL on every save and listen through the
// [outboxsql.Listener] given to [OutboxBuilder.UseSQLListener]. The poll
// schedule and an injected scheduler are required for both push watch modes.
func (b *OutboxBuilder) Build(ctx context.Context, handler outbox.Handler) (*outbox.Outbox, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if !b.cfg.Enabled {
		return nil, nil //nolint:nilnil // a disabled outbox is not an error
	}

	cfg := *b.cfg
	if cfg.Storage != nil {
		storage := *cfg.Storage
		cfg.Storage = &storage
	}
	cfg.Normalize()
	// Validate against this builder while preserving the config passed to storage functions.
	schema := cfg
	storage := *cfg.Storage
	storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	schema.Storage = &storage
	// Keep the normalized configuration local to this build.
	builder := *b
	builder.cfg = &cfg
	b = &builder
	if err := errors.Join(schema.ValidateStorage(), b.validateWatchBackend()); err != nil {
		return nil, fmt.Errorf("validate outbox storage config: %w", err)
	}

	watch := cfg.Watch == outboxconfig.WatchChangeStream || cfg.Watch == outboxconfig.WatchNotify
	if watch && b.scheduler == nil {
		return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "scheduler is required when watch is %q", cfg.Watch)
	}

	store, err := b.buildStore(ctx)
	if err != nil {
		return nil, err
	}

	if watch {
		if watcher, ok := store.(outbox.Watcher); !ok || nilcheck.IsNil(watcher) {
			return nil, unsupportedWatch(&cfg)
		}
	}

	ob, err := b.createOutboxWithStore(ctx, store, handler)
	if err == nil && watch {
		stop := ob.StartWatch(context.WithoutCancel(ctx))
		b.onShutdown(func(context.Context) error {
			stop()
			return nil
		})
	}
	return ob, err
}

// validateWatchBackend rejects incompatible built-in modes before provisioning storage.
func (b *OutboxBuilder) validateWatchBackend() error {
	cfg := b.cfg
	if cfg.Watch != outboxconfig.WatchChangeStream && cfg.Watch != outboxconfig.WatchNotify {
		return nil
	}
	switch cfg.Storage.Type {
	case outboxconfig.StorageTypeMongo:
		if cfg.Watch != outboxconfig.WatchChangeStream {
			return unsupportedWatch(cfg)
		}
	case outboxconfig.StorageTypeSQL:
		if cfg.Watch != outboxconfig.WatchNotify || cfg.Storage.SQL == nil || cfg.Storage.SQL.Dialect != outboxconfig.SQLDialectPostgres {
			return unsupportedWatch(cfg)
		}
	default:
		if _, registered := b.storages[cfg.Storage.Type]; !registered {
			return unsupportedWatch(cfg)
		}
	}
	return nil
}

func unsupportedWatch(cfg *outboxconfig.Config) error {
	return corefactory.Errorf(coreerrs.ErrInvalidArgument, "outbox watch %q with storage %q: %w",
		cfg.Watch, cfg.Storage.Type, outboxconfig.ErrUnsupportedWatch)
}

// buildStore constructs the [outbox.Storage] named by the storage type: the
// built-in sqldb store, or the one registered with [OutboxBuilder.UseStorage].
func (b *OutboxBuilder) buildStore(ctx context.Context) (outbox.Storage, error) {
	typ := b.cfg.Storage.Type
	if typ == outboxconfig.StorageTypeSQL {
		return b.buildSQLStore(ctx, b.cfg.Storage.SQL)
	}
	if build, ok := b.storages[typ]; ok {
		store, err := build(ctx, b.cfg, b.Logger())
		if err != nil {
			return nil, b.WrapError(err, "failed to create outbox store")
		}
		return store, nil
	}
	if typ == outboxconfig.StorageTypeMongo {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "outbox storage %q is not registered: add "+
			"factory/reliability/outbox/storages/mongo with UseStorage(%q, outboxmongofactory.Builder(db))", typ, typ)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported outbox storage type %q", typ)
}

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist.
func (b *OutboxBuilder) buildSQLStore(ctx context.Context, cfg *outboxconfig.SQLStorage) (*outboxsql.Store, error) {
	if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
		return nil, err
	}
	notify := b.cfg.Watch == outboxconfig.WatchNotify
	if notify {
		if err := b.RequireDependency(b.sqlListener, "SQL listener"); err != nil {
			return nil, err
		}
	}

	opts := []outboxsql.Option{
		outboxsql.WithTxResolver(b.resolveTx),
		outboxsql.WithTableName(cfg.Table), outboxsql.WithLogger(b.Logger())}
	opts = slices.AppendIf(opts, cfg.KeyOrder, outboxsql.WithKeyOrder())
	opts = slices.AppendIf(opts, cfg.AsynchronousCommit, outboxsql.WithAsynchronousCommit())
	opts = slices.AppendIf(opts, notify, outboxsql.WithNotify(), outboxsql.WithListener(b.sqlListener))
	opts = slices.AppendIf(opts, notify && cfg.NotifyChannel != "", outboxsql.WithNotifyChannel(cfg.NotifyChannel))
	store, err := outboxsql.New(b.sqlDB, outboxsql.Dialect(cfg.Dialect), opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create outbox store")
	}
	if cfg.EnsureSchema {
		schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureSchemaTimeout)
		defer cancel()
		if err := store.EnsureSchema(schemaCtx); err != nil {
			return nil, b.WrapError(err, "ensure outbox schema")
		}
	}
	return store, nil
}

// ensureSchemaTimeout bounds the schema creation buildSQLStore runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// onShutdown registers hook in the builder's scope, defaulting to the
// process-wide registry.
func (b *OutboxBuilder) onShutdown(hook coreruntime.ShutdownHook) {
	if b.shutdownHooks != nil {
		b.shutdownHooks.OnShutdown(hook)
		return
	}
	coreruntime.OnShutdown(hook)
}

// createOutboxWithStore creates an outbox with the given store (shared logic).
func (b *OutboxBuilder) createOutboxWithStore(
	ctx context.Context, store outbox.Storage, handler outbox.Handler,
) (*outbox.Outbox, error) {
	opts := b.buildOutboxOptions()

	// Add scheduler and schedule options if scheduler is available.
	// Task IDs go on every time (not gated by the schedule strings)
	// because the runtime collision check inspects all four IDs even
	// when some schedules are empty — keeping the IDs in lockstep with
	// their YAML counterparts means a config change that flips a
	// schedule on inherits the operator's overrides automatically.
	if b.scheduler != nil {
		opts = append(opts,
			outbox.WithScheduler(b.scheduler),
			outbox.WithDispatchTaskID(b.cfg.DispatchTaskID),
			outbox.WithUnlockTaskID(b.cfg.UnlockTaskID),
			outbox.WithExpireTaskID(b.cfg.ExpireTaskID),
			outbox.WithCleanupTaskID(b.cfg.CleanupTaskID),
			outbox.WithStatsTaskID(b.cfg.StatsTaskID),
		)
		opts = slices.AppendIf(opts, b.cfg.DispatchSchedule != "", outbox.WithDispatchSchedule(b.cfg.DispatchSchedule))
		opts = slices.AppendIf(opts, b.cfg.UnlockSchedule != "", outbox.WithUnlockSchedule(b.cfg.UnlockSchedule))
		opts = slices.AppendIf(opts, b.cfg.CleanupSchedule != "", outbox.WithCleanupSchedule(b.cfg.CleanupSchedule))
		opts = slices.AppendIf(opts, b.cfg.ExpireSchedule != "", outbox.WithExpireSchedule(b.cfg.ExpireSchedule))
		opts = slices.AppendIf(opts, b.cfg.StatsSchedule != "", outbox.WithStatsSchedule(b.cfg.StatsSchedule))
	}

	opts = append(opts, b.outboxOpts...)
	ob := outbox.New(store, handler, opts...)
	if err := ob.RegisterTasks(ctx); err != nil {
		return ob, b.WrapError(err, "register outbox tasks")
	}
	return ob, nil
}

// buildOutboxOptions builds outbox options from configuration.
func (b *OutboxBuilder) buildOutboxOptions() []outbox.Option {
	opts := []outbox.Option{
		outbox.WithLogger(b.Logger()),
		outbox.WithFetchTimeout(b.cfg.FetchTimeout),
		outbox.WithHandleTimeout(b.cfg.HandleTimeout),
		outbox.WithUpdateTimeout(b.cfg.UpdateTimeout),
		outbox.WithEventsBatchSize(b.cfg.MessagesBatchSize),
		outbox.WithRetryMaxAttempts(b.cfg.RetryMaxAttempts),
		outbox.WithRetryBaseDelay(b.cfg.RetryBaseDelay),
		outbox.WithRetryMaxDelay(b.cfg.RetryMaxDelay),
		outbox.WithMaxLockTime(b.cfg.MaxLockTime),
		outbox.WithMaxPayloadBytes(b.cfg.MaxPayloadBytes),
		outbox.WithPublishedEventsLifetime(b.cfg.PublishedEventsLifetime),
		outbox.WithDefaultEventTTL(b.cfg.DefaultEventTTL),
	}
	opts = slices.AppendIf(opts, b.cfg.TopicCompaction, outbox.WithCompaction())

	return opts
}
