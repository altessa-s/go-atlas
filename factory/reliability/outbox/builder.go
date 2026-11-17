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
	"github.com/altessa-s/go-atlas/reliability/outbox"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// StorageFunc creates the outbox storage from the outbox configuration. ctx
// bounds the storage's startup I/O (index or schema creation); logger is the
// builder's logger.
type StorageFunc func(ctx context.Context, cfg *brokerconfig.Outbox, logger *slog.Logger) (outbox.Storage, error)

// OutboxBuilder assembles an [outbox.Outbox] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at build time.
// The builder is not safe for concurrent use.
//
// The storage is selected by brokerconfig.Outbox.Storage.Type. The builder
// builds the sqldb store itself over the handle given to
// [OutboxBuilder.UseSQLDB]. The MongoDB one registers its [StorageFunc] with
// [OutboxBuilder.UseStorage] from factory/reliability/outbox/storages/mongo; a
// configured type without one fails at Build naming the package to add.
type OutboxBuilder struct {
	corefactory.Base
	cfg  *brokerconfig.Outbox
	errs []error

	// Dependencies
	scheduler     schedtask.Registrar
	outboxOpts    []outbox.Option
	storages      map[brokerconfig.OutboxStorageType]StorageFunc
	sqlDB         *sql.DB
	sqlListener   outboxsql.Listener
	shutdownHooks *coreruntime.HookGroup
}

// New creates a new [OutboxBuilder] for the given outbox config.
// Config can be nil -- the error surfaces at build time.
func New(cfg *brokerconfig.Outbox) *OutboxBuilder {
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
// With watch set to changeStream or notify, the builder starts
// [outbox.Outbox.Watch] in its own goroutine once the tasks are registered,
// and stops it through the shutdown hook group (the process registry unless
// [OutboxBuilder.UseShutdownHooks] says otherwise). The notify mode makes the
// sqldb store signal PostgreSQL on every save and listen through the
// [outboxsql.Listener] given to [OutboxBuilder.UseSQLListener]. The poll
// schedule keeps running in every mode.
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

	b.cfg.Normalize()
	if err := b.cfg.ValidateStorage(); err != nil {
		return nil, fmt.Errorf("validate outbox storage config: %w", err)
	}

	store, err := b.buildStore(ctx)
	if err != nil {
		return nil, err
	}

	ob, err := b.createOutboxWithStore(ctx, store, handler)
	if err != nil {
		return ob, err
	}
	if b.cfg.Watch == brokerconfig.OutboxWatchChangeStream || b.cfg.Watch == brokerconfig.OutboxWatchNotify {
		b.startWatch(ctx, ob)
	}
	return ob, nil
}

// buildStore constructs the [outbox.Storage] named by the storage type: the
// built-in sqldb store, or the one registered with [OutboxBuilder.UseStorage].
func (b *OutboxBuilder) buildStore(ctx context.Context) (outbox.Storage, error) {
	switch typ := b.cfg.Storage.Type; typ {
	case brokerconfig.OutboxStorageTypeSQL:
		return b.buildSQLStore(ctx, b.cfg.Storage.SQL)
	case brokerconfig.OutboxStorageTypeMongo:
		build, ok := b.storages[typ]
		if !ok {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "outbox storage %q is not registered: add "+
				"factory/reliability/outbox/storages/mongo with UseStorage(%q, outboxmongofactory.Builder(db))", typ, typ)
		}
		store, err := build(ctx, b.cfg, b.Logger())
		if err != nil {
			return nil, b.WrapError(err, "failed to create outbox store")
		}
		return store, nil
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported outbox storage type %q", typ)
	}
}

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist.
func (b *OutboxBuilder) buildSQLStore(ctx context.Context, cfg *brokerconfig.OutboxSQLStorageConfig) (*outboxsql.Store, error) {
	if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
		return nil, err
	}
	notify := b.cfg.Watch == brokerconfig.OutboxWatchNotify
	if notify {
		if err := b.RequireDependency(b.sqlListener, "SQL listener"); err != nil {
			return nil, err
		}
	}

	opts := []outboxsql.Option{outboxsql.WithTableName(cfg.Table), outboxsql.WithLogger(b.Logger())}
	opts = slices.AppendIf(opts, notify, outboxsql.WithNotify(), outboxsql.WithListener(b.sqlListener))
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

// startWatch runs [outbox.Outbox.Watch] until shutdown. The watcher outlives
// Build, so it runs on a context detached from the build context's
// cancellation; the shutdown hook cancels it and waits for it to return.
func (b *OutboxBuilder) startWatch(ctx context.Context, ob *outbox.Outbox) {
	logger := b.Logger()
	watchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})

	go func() {
		defer close(done)
		switch err := ob.Watch(watchCtx); {
		case err == nil:
		case errors.Is(err, outbox.ErrWatchUnsupported):
			logger.WarnContext(watchCtx, "outbox store cannot push notifications; dispatch stays on the poll schedule",
				slog.String("watch", string(b.cfg.Watch)))
		default:
			logger.ErrorContext(watchCtx, "outbox watcher stopped", slog.Any("error", err))
		}
	}()

	b.onShutdown(func(ctx context.Context) error {
		cancel()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

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
