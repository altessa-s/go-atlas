// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/reliability/saga"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	memorystore "github.com/altessa-s/go-atlas/reliability/saga/storages/memory"
	sqlstore "github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// StorageFunc creates the saga state store of one storage type from the saga
// storage configuration. ctx bounds the startup I/O (bucket and index
// creation).
type StorageFunc func(ctx context.Context, cfg *sagaconfig.StorageConfig) (saga.Storage, error)

// Builder assembles a [saga.Orchestrator] from a [sagaconfig.Config] and injected
// backend clients, using a fluent API. Create instances with [New]. The state
// store is selected by sagaconfig.Config.Storage.Type.
//
// The builder builds only the memory store and, with [Builder.UseSQLDB], the
// sqldb one. The NATS, MongoDB and Redis stores register their [StorageFunc]
// with [Builder.UseStorage] from reliability/saga/storages/<backend>/factory; a
// configured type without one fails at Build naming the package to add.
//
// The builder is generic over the saga's shared data type T so it can return a
// fully typed orchestrator. It is not safe for concurrent use.
type Builder[T any] struct {
	corefactory.Base
	cfg  *sagaconfig.Config
	def  *saga.Definition[T]
	errs []error

	storages map[sagaconfig.StorageType]StorageFunc
	sqlDB    *sql.DB

	// Optional orchestrator dependencies.
	collector     metrics.Meter
	scheduler     schedtask.Registrar
	leaderElector saga.LeaderElector
	serializer    serializer.Serializer
	onDeadLetter  saga.DeadLetterFunc
	shouldRetry   func(error) bool
}

// New creates a Builder for the given saga config and definition. Both may be
// nil — the error surfaces at [Builder.Build] time.
func New[T any](cfg *sagaconfig.Config, def *saga.Definition[T]) *Builder[T] {
	return &Builder[T]{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
		def:  def,
	}
}

// Build validates the configuration, constructs the configured state store
// (built in or registered with [Builder.UseStorage]), and returns a typed orchestrator.
// ctx bounds the startup I/O (SQL schema creation, recovery registration).
func (b *Builder[T]) Build(ctx context.Context) (*saga.Orchestrator[T], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.def == nil {
		return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "definition is required")
	}

	cfg := *b.cfg
	if cfg.Storage != nil {
		storage := *cfg.Storage
		cfg.Storage = &storage
	}
	cfg.Normalize()
	// Keep the normalized configuration local to this build.
	builder := *b
	builder.cfg = &cfg
	b = &builder
	if err := b.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate saga config: %w", err)
	}

	store, err := b.buildStore(ctx)
	if err != nil {
		return nil, err
	}

	o := saga.New(store, b.def, b.orchestratorOptions()...)
	if err := o.RegisterRecovery(ctx); err != nil {
		return nil, fmt.Errorf("register saga recovery: %w", err)
	}
	return o, nil
}

// buildStore constructs the [saga.Storage] named by the storage type: the
// built-in memory or sqldb store, or the one registered with [Builder.UseStorage].
func (b *Builder[T]) buildStore(ctx context.Context) (saga.Storage, error) {
	storage := b.cfg.Storage
	switch storage.Type {
	case sagaconfig.StorageTypeMemory:
		return memorystore.New(), nil
	case sagaconfig.StorageTypeSQL:
		if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
			return nil, err
		}
		return b.buildSQLStore(ctx, storage.SQL)
	case sagaconfig.StorageTypeNATS:
		return b.buildRegisteredStore(ctx, storage, "js")
	case sagaconfig.StorageTypeMongo:
		return b.buildRegisteredStore(ctx, storage, "db")
	case sagaconfig.StorageTypeRedis:
		return b.buildRegisteredStore(ctx, storage, "client")
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported saga storage type %q", storage.Type)
	}
}

// buildRegisteredStore runs the [StorageFunc] registered for the storage type;
// without one it names the package to add and the client its Builder takes.
func (b *Builder[T]) buildRegisteredStore(ctx context.Context, storage *sagaconfig.StorageConfig, client string) (saga.Storage, error) {
	if build, ok := b.storages[storage.Type]; ok {
		return build(ctx, storage)
	}
	return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "saga storage %q is not registered: add factory/reliability/saga/storages/%s "+
		"with UseStorage(%q, %sfactory.Builder(%s))", storage.Type, storage.Type, storage.Type, storage.Type, client)
}

// ensureSchemaTimeout bounds the schema creation buildSQLStore runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist.
func (b *Builder[T]) buildSQLStore(ctx context.Context, cfg *sagaconfig.SQLStorageConfig) (*sqlstore.Store, error) {
	store, err := sqlstore.New(b.sqlDB, sqlstore.Dialect(cfg.Dialect), sqlstore.WithTableName(cfg.Table))
	if err != nil || !cfg.EnsureSchema {
		return store, err
	}
	schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureSchemaTimeout)
	defer cancel()
	if err := store.EnsureSchema(schemaCtx); err != nil {
		return nil, b.WrapError(err, "ensure saga schema")
	}
	return store, nil
}

// orchestratorOptions translates the config and injected dependencies into
// orchestrator options. The generated With* options are nil/zero-guarded, so
// unset dependencies and zero values are safely ignored.
func (b *Builder[T]) orchestratorOptions() []saga.Option {
	cfg := b.cfg
	return []saga.Option{
		saga.WithLogger(b.Logger()),
		saga.WithMeter(b.collector),
		saga.WithScheduler(b.scheduler),
		saga.WithLeaderElector(b.leaderElector),
		saga.WithSerializer(b.serializer),
		saga.WithOnDeadLetter(b.onDeadLetter),
		saga.WithShouldRetry(b.shouldRetry),
		saga.WithStepTimeout(cfg.StepTimeout),
		saga.WithExecutionTimeout(cfg.ExecutionTimeout),
		saga.WithLeaseGrace(cfg.LeaseGrace),
		saga.WithStorageTimeout(cfg.StorageTimeout),
		saga.WithRecoveryTimeout(cfg.RecoveryTimeout),
		saga.WithSagaTimeout(cfg.SagaTimeout),
		saga.WithMaxStepAttempts(cfg.MaxStepAttempts),
		saga.WithStepRetryBaseDelay(cfg.StepRetryBaseDelay),
		saga.WithStepRetryMaxDelay(cfg.StepRetryMaxDelay),
		saga.WithMaxCompensationAttempts(cfg.MaxCompensationAttempts),
		saga.WithStepConcurrency(cfg.StepConcurrency),
		saga.WithRecoverySchedule(cfg.RecoverySchedule),
		saga.WithRecoveryBatchSize(cfg.RecoveryBatchSize),
		saga.WithRecoveryTaskID(cfg.RecoveryTaskID),
	}
}
