// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/keyset"
	"github.com/altessa-s/go-atlas/service/dispatch"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
	auditclickhouse "github.com/altessa-s/go-atlas/data/audit/storages/clickhouse"
	memorystorage "github.com/altessa-s/go-atlas/data/audit/storages/memory"
	mongostorage "github.com/altessa-s/go-atlas/data/audit/storages/mongo"
	dispatchfactory "github.com/altessa-s/go-atlas/service/dispatch/factory"
)

// ErrDisabled is returned by [AuditorBuilder.Build] when auditing is disabled
// in the configuration. Callers should use [errors.Is] to check for this sentinel.
var ErrDisabled = errors.New("audit: disabled by configuration")

// ErrMongoDatabaseRequired is returned by [AuditorBuilder.Build] when the
// configured storage type is "mongo" but no database was supplied via
// [AuditorBuilder.UseMongoDatabase].
var ErrMongoDatabaseRequired = errors.New("audit: mongo database is required for storage type mongo")

// ErrClickHouseConnRequired is returned by Build and BuildStorage when the
// configured storage type is "clickhouse" but no connection was supplied via
// [AuditorBuilder.UseClickHouseConn].
var ErrClickHouseConnRequired = errors.New("audit: clickhouse connection is required for storage type clickhouse")

// MinRecommendedClickHouseBatchSize is the smallest dispatch batch size the
// builder accepts without a warning for ClickHouse: every flush creates a part
// the server has to merge.
const MinRecommendedClickHouseBatchSize = 1000

// AuditorBuilder assembles an [audit.Auditor] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [AuditorBuilder.Build] time.
// The builder is not safe for concurrent use.
type AuditorBuilder struct {
	corefactory.Base
	cfg  *auditconfig.Config
	errs []error

	// Dependencies
	dispatcher     audit.Dispatcher
	mongoDatabase  *mongo.Database
	clickhouseConn auditclickhouse.Conn
	shutdownHooks  *coreruntime.HookGroup
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
// and reported here via [errors.Join].
//
// Returns (nil, [ErrDisabled]) when the configuration has Enabled set to false.
//
// When no dispatcher is injected via [AuditorBuilder.UseDispatcher], the builder
// creates one from the configuration: it resolves `storage` into an
// [audit.Storage] and wraps it in a [dispatch.Engine] configured by `dispatch`.
// The engine it owns is shut down through the builder's hook group (the process
// registry unless [AuditorBuilder.UseShutdownHooks] says otherwise), bounded by
// `shutdownTimeout`.
func (b *AuditorBuilder) Build() (*audit.Auditor, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !b.cfg.Enabled {
		return nil, ErrDisabled
	}

	dispatcher := b.dispatcher
	if dispatcher == nil {
		engine, err := b.createEngine()
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
func (b *AuditorBuilder) createEngine() (*dispatch.Engine[*audit.Event], error) {
	storage, err := b.createStorage()
	if err != nil {
		return nil, err
	}

	engine, err := dispatchfactory.New[*audit.Event](&b.cfg.Dispatch).
		WithSink(audit.StorageSink{Storage: storage}).
		WithCodec(audit.JSONCodec{}).
		WithLogger(b.Logger()).
		Build()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit dispatch engine")
	}

	b.onShutdown(func(ctx context.Context) error {
		// The engine drains in-flight batches on shutdown; ShutdownTimeout is
		// how long the caller is willing to wait for that drain.
		ctx, cancel := b.shutdownContext(ctx)
		defer cancel()

		return engine.Shutdown(ctx)
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
func (b *AuditorBuilder) createStorage() (audit.Storage, error) {
	switch b.cfg.Storage.Type {
	case auditconfig.StorageTypeMemory:
		return memorystorage.New(), nil

	case auditconfig.StorageTypeMongo:
		return b.createMongoStorage()

	case auditconfig.StorageTypeClickHouse:
		return b.createClickHouseStorage()

	default:
		return nil, coreerrs.Wrapf(
			fmt.Errorf("unsupported storage type: %s", b.cfg.Storage.Type),
			"invalid configuration for %s", "audit storage")
	}
}

// createMongoStorage builds the MongoDB-backed storage. The nested `mongo`
// section is optional — its absence means "all defaults", not an error, because
// the storage package supplies its own.
func (b *AuditorBuilder) createMongoStorage() (audit.Storage, error) {
	if b.mongoDatabase == nil {
		return nil, ErrMongoDatabaseRequired
	}

	var opts []mongostorage.Option
	if cfg := b.cfg.Storage.Mongo; cfg != nil {
		opts = coreslices.AppendIf(opts, cfg.CollectionName != "",
			mongostorage.WithCollectionName(cfg.CollectionName))
		opts = coreslices.AppendIf(opts, cfg.IndexTimeout > 0,
			mongostorage.WithIndexTimeout(cfg.IndexTimeout))
		opts = coreslices.AppendIf(opts, cfg.TTL > 0,
			mongostorage.WithTTL(cfg.TTL))
	}

	storage, err := mongostorage.New(b.mongoDatabase, opts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit mongo storage")
	}

	return storage, nil
}

// createClickHouseStorage builds the ClickHouse-backed storage. Missing
// columns and indexes are added when the configuration opts in, then the
// table is checked against the expected schema, so drift surfaces at startup.
func (b *AuditorBuilder) createClickHouseStorage() (audit.Storage, error) {
	if b.clickhouseConn == nil {
		return nil, ErrClickHouseConnRequired
	}
	cfg := b.cfg.Storage.ClickHouse
	if cfg == nil {
		return nil, fmt.Errorf("clickhouse storage configuration is required")
	}

	if size := b.cfg.Dispatch.BatchSize; size > 0 && size < MinRecommendedClickHouseBatchSize {
		b.Logger().Warn("audit dispatch batch size is small for ClickHouse: every flush creates a part to merge",
			"batchSize", size, "recommended", MinRecommendedClickHouseBatchSize)
	}

	opts := []auditclickhouse.Option{
		auditclickhouse.WithTableName(cfg.TableName),
		auditclickhouse.WithEngine(cfg.Engine),
		auditclickhouse.WithCluster(cfg.Cluster),
		auditclickhouse.WithMaxBatchSize(cfg.MaxBatchSize),
		auditclickhouse.WithLogger(b.Logger()),
	}
	opts = coreslices.AppendIf(opts, cfg.TimeRangeMode != "",
		auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeMode(cfg.TimeRangeMode)))
	opts = coreslices.AppendIf(opts, cfg.SchemaCheckMode != "",
		auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckMode(cfg.SchemaCheckMode)))
	opts = coreslices.AppendIf(opts, cfg.SchemaMigrationMode != "",
		auditclickhouse.WithSchemaMigration(auditclickhouse.SchemaMigrationMode(cfg.SchemaMigrationMode)))
	opts = coreslices.AppendIf(opts, cfg.TTL > 0, auditclickhouse.WithTTL(cfg.TTL))
	opts = coreslices.AppendIf(opts, cfg.DDLTimeout > 0, auditclickhouse.WithDDLTimeout(cfg.DDLTimeout))
	opts = coreslices.AppendIf(opts, cfg.AutoCreateTable, auditclickhouse.WithAutoCreateTable())
	opts = coreslices.AppendIf(opts, cfg.Final, auditclickhouse.WithFinal())

	storage, err := auditclickhouse.New(b.clickhouseConn, opts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create audit clickhouse storage")
	}

	// Migration runs first so the check reports only what was left
	// unrepaired; it is a no-op unless the configuration opts in. Each step
	// gets its own DDLTimeout, so a hung server fails startup instead of
	// blocking it.
	timeout := cmp.Or(cfg.DDLTimeout, auditclickhouse.DefaultDDLTimeout)
	for _, step := range []func(context.Context) error{storage.MigrateSchema, storage.CheckSchema} {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := step(ctx)
		cancel()
		if err != nil {
			return nil, err
		}
	}

	return storage, nil
}

// BuildStorage returns the configured [audit.Storage] for reading events,
// for example with [audit.FetchPage]. It does not start a dispatcher; the
// caller owns the storage and closes it.
func (b *AuditorBuilder) BuildStorage() (audit.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	return b.createStorage()
}

// BuildPageTokens returns the page tokens for [audit.FetchPage] from the
// `paging` configuration, or nil when paging is not configured.
func (b *AuditorBuilder) BuildPageTokens() (*audit.PageTokens, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	paging := b.cfg.Paging
	if paging == nil {
		return nil, nil //nolint:nilnil // nil tokens disable paging
	}

	previous := make([][]byte, len(paging.PreviousKeys))
	for i, k := range paging.PreviousKeys {
		previous[i] = []byte(k.Expose())
	}
	opts := []keyset.Option{keyset.WithPreviousKeys(previous...)}
	opts = coreslices.AppendIf(opts, paging.TokenTTL > 0, keyset.WithTTL(paging.TokenTTL))

	codec, err := keyset.New([]byte(paging.SigningKey.Expose()), opts...)
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
	if b.shutdownHooks != nil {
		auditorOpts = append(auditorOpts, audit.WithShutdownHooks(b.shutdownHooks))
	}

	auditor, err := audit.New(dispatcher, auditorOpts...)
	if err != nil {
		return nil, fmt.Errorf("create auditor: %w", err)
	}

	if err := auditor.Start(); err != nil {
		return nil, fmt.Errorf("start auditor: %w", err)
	}

	return auditor, nil
}
