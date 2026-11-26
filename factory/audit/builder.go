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

	"github.com/altessa-s/go-atlas-core/runtime/dispatch"
	"github.com/altessa-s/go-atlas/audit"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	memorystorage "github.com/altessa-s/go-atlas/audit/storages/memory"
	auditsql "github.com/altessa-s/go-atlas/audit/storages/sqldb"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	pagetokenfactory "github.com/altessa-s/go-atlas/factory/ext/aip/pagination/pagetoken"
	dispatchfactory "github.com/altessa-s/go-atlas/factory/ext/core/dispatch"
)

// ErrDisabled is returned by [AuditorBuilder.Build] when auditing is disabled
// in the configuration. Callers should use [errors.Is] to check for this sentinel.
var ErrDisabled = errors.New("audit: disabled by configuration")

// StorageBuilder creates the audit storage of one storage type from the audit
// configuration. ctx bounds the storage's startup I/O (schema creation and
// checks); logger is the builder's.
type StorageBuilder func(ctx context.Context, cfg *auditconfig.Config, logger *slog.Logger) (audit.Storage, error)

// AuditorBuilder assembles an [audit.Auditor] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [AuditorBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds the memory and sqldb storages. The MongoDB and ClickHouse
// ones register their [StorageBuilder] with [AuditorBuilder.UseStorage] from
// factory/audit/storages/mongo and factory/audit/storages/clickhouse;
// a configured type without one fails at Build naming the package to add.
type AuditorBuilder struct {
	corefactory.Base
	cfg  *auditconfig.Config
	errs []error

	// Dependencies
	dispatcher    audit.Dispatcher
	sqlDB         *sql.DB
	resolveTx     func(context.Context) (*sql.Tx, bool, error)
	shutdownHooks *coreruntime.HookGroup
	storages      map[auditconfig.StorageType]StorageBuilder
}

// New creates an [AuditorBuilder] for the given audit config.
// Config can be nil — the error surfaces at [AuditorBuilder.Build] time.
func New(cfg *auditconfig.Config) *AuditorBuilder {
	return &AuditorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the audit Auditor. Errors from fluent methods are accumulated
// and reported here via [errors.Join]. ctx bounds the startup I/O of the
// storage (schema creation and checks).
//
// Returns (nil, [ErrDisabled]) when the configuration has Enabled set to false.
//
// When no dispatcher is injected via [AuditorBuilder.UseDispatcher], the builder
// creates one from the configuration: it resolves `storage` into an
// [audit.Storage] and wraps it in a [dispatch.Engine] configured by `dispatch`.
// The engine it owns is shut down through the builder's hook group (the process
// registry unless [AuditorBuilder.UseShutdownHooks] says otherwise), bounded by
// `shutdownTimeout`.
func (b *AuditorBuilder) Build(ctx context.Context) (*audit.Auditor, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if !b.cfg.Enabled {
		return nil, ErrDisabled
	}

	dispatcher := b.dispatcher
	if dispatcher == nil {
		engine, err := b.createEngine(ctx)
		if err != nil {
			return nil, err
		}
		dispatcher = engine
	}

	return b.createAuditor(dispatcher)
}

// createEngine builds and starts a [dispatch.Engine] over the configured
// storage, and arranges for it to be stopped.
//
// The engine is the builder's to own, so the builder is also the only place
// that can stop it — an engine nobody shuts down keeps its workers and, with
// WAL enabled, its segment files open past the point the caller believes the
// subsystem is gone.
func (b *AuditorBuilder) createEngine(ctx context.Context) (*dispatch.Engine[*audit.Event], error) {
	storage, err := b.createStorage(ctx)
	if err != nil {
		return nil, err
	}

	engine, err := dispatchfactory.New[*audit.Event](&b.cfg.Dispatch).
		WithSink(audit.StorageSink{Storage: storage}).
		WithCodec(audit.JSONCodec{}).
		WithLogger(b.Logger()).
		Build()
	if err != nil {
		_ = storage.Close(ctx)
		return nil, coreerrs.WrapOperation(err, "create audit dispatch engine")
	}

	b.onShutdown(func(ctx context.Context) error {
		// The engine drains in-flight batches on shutdown; ShutdownTimeout is
		// how long the caller is willing to wait for that drain.
		ctx, cancel := b.shutdownContext(ctx)
		defer cancel()

		return errors.Join(engine.Shutdown(ctx), storage.Close(ctx))
	})

	return engine, nil
}

// shutdownContext applies the configured shutdown timeout to ctx. A zero
// timeout leaves the caller's own deadline in charge.
func (b *AuditorBuilder) shutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if b.cfg.ShutdownTimeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, b.cfg.ShutdownTimeout)
}

// onShutdown registers hook in the builder's scope, defaulting to the
// process-wide registry.
func (b *AuditorBuilder) onShutdown(hook coreruntime.ShutdownHook) {
	if b.shutdownHooks != nil {
		b.shutdownHooks.OnShutdown(hook)
		return
	}

	coreruntime.OnShutdown(hook)
}

// createStorage resolves the configured storage type into an [audit.Storage].
func (b *AuditorBuilder) createStorage(ctx context.Context) (audit.Storage, error) {
	if b.cfg.Storage.Type == auditconfig.StorageTypeMemory {
		return memorystorage.New(), nil
	}
	if b.cfg.Storage.Type == auditconfig.StorageTypeSQL {
		return b.createSQLStorage(ctx)
	}
	if build, ok := b.storages[b.cfg.Storage.Type]; ok && b.cfg.Storage.Type != "" {
		return build(ctx, b.cfg, b.Logger())
	}
	switch b.cfg.Storage.Type {
	case auditconfig.StorageTypeMongo:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "audit storage %q is not registered: add factory/audit/storages/mongo "+
			"with UseStorage(%q, mongofactory.Builder(db))", b.cfg.Storage.Type, b.cfg.Storage.Type)
	case auditconfig.StorageTypeClickHouse:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "audit storage %q is not registered: add factory/audit/storages/clickhouse "+
			"with UseStorage(%q, chfactory.Builder(conn))", b.cfg.Storage.Type, b.cfg.Storage.Type)
	default:
		return nil, coreerrs.Wrapf(
			corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", b.cfg.Storage.Type),
			"invalid configuration for %s", "audit storage")
	}
}

// ErrSQLDBRequired is returned by [AuditorBuilder.Build] when the configured
// storage type is "sqldb" but no database was supplied via
// [AuditorBuilder.UseSQLDB].
var ErrSQLDBRequired = corefactory.Errorf(corefactory.ErrDependencyRequired, "audit: sql database is required for storage type sqldb")

// ensureSchemaTimeout bounds the schema creation createSQLStorage runs when
// the config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// createSQLStorage builds the SQL-backed storage. With EnsureSchema set it
// creates the table through the storage's idempotent EnsureSchema; otherwise
// it performs no I/O and the schema is expected to exist.
func (b *AuditorBuilder) createSQLStorage(ctx context.Context) (audit.Storage, error) {
	if b.sqlDB == nil {
		return nil, ErrSQLDBRequired
	}
	cfg := b.cfg.Storage.SQL
	if cfg == nil {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "sql storage configuration is required")
	}
	opts := []auditsql.Option{
		auditsql.WithTxResolver(b.resolveTx),
		auditsql.WithTableName(cfg.TableName), auditsql.WithTTL(cfg.TTL), auditsql.WithLogger(b.Logger()),
		auditsql.WithPurgeInterval(cfg.PurgeInterval), auditsql.WithPurgeBatchSize(cfg.PurgeBatchSize)}
	opts = coreslices.AppendIf(opts, cfg.MaxBatchRows > 0, auditsql.WithMaxBatchRows(cfg.MaxBatchRows))
	// Retention runs until Close; ctx only bounds schema startup.
	storage, err := auditsql.New(b.sqlDB, auditsql.Dialect(cfg.Dialect), opts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit sql storage")
	}
	if cfg.EnsureSchema {
		schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureSchemaTimeout)
		defer cancel()
		if err := storage.EnsureSchema(schemaCtx); err != nil {
			_ = storage.Close(ctx)
			return nil, coreerrs.WrapOperation(err, "create audit sql schema")
		}
	}
	return storage, nil
}

// BuildStorage returns the configured [audit.Storage] for reading events,
// for example with [audit.FetchPage]. It does not start a dispatcher; the
// caller may close it directly; SQL storage is also registered for shutdown.
// ctx bounds the storage startup I/O (schema creation and checks).
func (b *AuditorBuilder) BuildStorage(ctx context.Context) (audit.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	storage, err := b.createStorage(ctx)
	if err != nil {
		return nil, err
	}
	// BuildStorage can also return a writable SQL storage. Cover its lazy loop
	// even when no dispatch engine owns the storage's shutdown.
	if b.cfg.Storage.Type == auditconfig.StorageTypeSQL {
		b.onShutdown(storage.Close)
	}
	return storage, nil
}

// BuildPageTokens returns the page tokens for [audit.FetchPage] from the
// `paging` configuration, or nil when paging is not configured.
func (b *AuditorBuilder) BuildPageTokens() (*audit.PageTokens, error) {
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if b.cfg.Paging == nil {
		return nil, nil //nolint:nilnil // nil tokens disable paging
	}

	codec, err := pagetokenfactory.New(b.cfg.Paging).Build()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit page tokens")
	}
	return audit.NewPageTokens(codec), nil
}

// createAuditor wraps the [audit.Dispatcher] with an Auditor and starts the
// facade. The dispatcher must already be started.
func (b *AuditorBuilder) createAuditor(dispatcher audit.Dispatcher) (*audit.Auditor, error) {
	auditorOpts := []audit.Option{
		audit.WithLogger(b.Logger()),
	}

	// Keep the auditor in the same shutdown scope as the engine it fronts, so
	// stopping the scope stops the whole subsystem rather than half of it.
	auditorOpts = coreslices.AppendIf(auditorOpts, b.shutdownHooks != nil, audit.WithShutdownHooks(b.shutdownHooks))

	auditor, err := audit.New(dispatcher, auditorOpts...)
	if err != nil {
		return nil, fmt.Errorf("create auditor: %w", err)
	}

	if err := auditor.Start(); err != nil {
		return nil, fmt.Errorf("start auditor: %w", err)
	}

	return auditor, nil
}
