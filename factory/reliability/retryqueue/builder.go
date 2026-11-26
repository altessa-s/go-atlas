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
	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	memorystore "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/memory"
	sqlstore "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
)

// StorageFunc creates the retry queue store of one storage type from the
// storage configuration. ctx bounds the startup I/O (index creation).
type StorageFunc[P any] func(ctx context.Context, cfg *retryqueueconfig.StorageConfig) (retryqueue.Storage[P], error)

// Builder assembles a [retryqueue.Storage] from a [retryqueueconfig.Config]
// and injected backend clients, using a fluent API. Create instances with
// [New]. The store is selected by retryqueueconfig.Config.Storage.Type.
//
// The builder builds only the memory store and, with [Builder.UseSQLDB], the
// sqldb one. The MongoDB store registers its [StorageFunc] with
// [Builder.UseStorage] from factory/reliability/retryqueue/storages/mongo; a
// configured type without one fails at Build naming the package to add.
//
// The builder is generic over the payload type P so it returns a fully typed
// store. Every bundled store also implements [retryqueue.Mover]. It is not
// safe for concurrent use.
type Builder[P any] struct {
	corefactory.Base
	cfg  *retryqueueconfig.Config
	errs []error

	storages     map[retryqueueconfig.StorageType]StorageFunc[P]
	sqlDB        *sql.DB
	resolveTx    func(context.Context) (*sql.Tx, bool, error)
	codec        serializer.Serializer
	clonePayload func(P) P

	shutdownHooks *coreruntime.HookGroup
}

// New creates a Builder for the given config. It may be nil — the error
// surfaces at [Builder.Build] time.
func New[P any](cfg *retryqueueconfig.Config) *Builder[P] {
	return &Builder[P]{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build validates the configuration and constructs the configured store
// (built in or registered with [Builder.UseStorage]). ctx bounds the startup
// I/O (SQL schema or MongoDB index creation). Every bundled store implements
// [retryqueue.AttemptStorage]; workers should check for that interface before
// processing item snapshots.
func (b *Builder[P]) Build(ctx context.Context) (retryqueue.Storage[P], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}
	cfg := *b.cfg
	if cfg.Storage != nil {
		st := *cfg.Storage
		cfg.Storage = &st
	}
	cfg.Normalize()
	// Validate against this builder while preserving the config passed to storage functions.
	schema := cfg
	if cfg.Storage != nil {
		storage := *cfg.Storage
		storage.SetRegisteredTypes(b.RegisteredStorageTypes())
		schema.Storage = &storage
	}
	if err := schema.Validate(); err != nil {
		return nil, fmt.Errorf("validate retry queue config: %w", err)
	}

	storage := cfg.Storage
	if storage.Type == retryqueueconfig.StorageTypeMemory {
		return memorystore.NewWithClone(b.clonePayload), nil
	}
	if storage.Type == retryqueueconfig.StorageTypeSQL {
		if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
			return nil, err
		}
		store, err := b.buildSQLStore(ctx, storage.SQL)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
	if build, ok := b.storages[storage.Type]; ok {
		return build(ctx, storage)
	}
	if storage.Type == retryqueueconfig.StorageTypeMongo {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "retry queue storage %q is not registered: add "+
			"factory/reliability/retryqueue/storages/mongo with UseStorage(%q, mongofactory.Builder[P](db))", storage.Type, storage.Type)
	}

	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported retry queue storage type %q", storage.Type)
}

// EnsureSchemaTimeout bounds the schema or index creation a storage runs at
// build when the config asks for it, so a lock wait or a stuck index build
// cannot stall startup indefinitely.
const EnsureSchemaTimeout = 30 * time.Second

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist. With a purge interval it registers
// the store's StopPurge as a shutdown hook to join the background purge.
func (b *Builder[P]) buildSQLStore(ctx context.Context, cfg *retryqueueconfig.SQLStorageConfig) (*sqlstore.Store[P], error) {
	store, err := sqlstore.New[P](b.sqlDB, sqlstore.Dialect(cfg.Dialect),
		sqlstore.WithTableName(cfg.Table),
		sqlstore.WithTxResolver(b.resolveTx),
		sqlstore.WithResolvedRetention(cfg.ResolvedRetention),
		sqlstore.WithCodec(b.codec),
		sqlstore.WithPurgeInterval(cfg.PurgeInterval),
		sqlstore.WithPurgeBatchSize(cfg.PurgeBatchSize),
		sqlstore.WithLogger(b.Logger()),
	)
	if err != nil {
		return nil, corefactory.Errorf(coreerrs.ErrInvalidArgument, "create retry queue SQL store: %w", err)
	}
	if cfg.EnsureSchema {
		schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, EnsureSchemaTimeout)
		defer cancel()
		if err := store.EnsureSchema(schemaCtx); err != nil {
			store.StopPurge()
			return nil, corefactory.Errorf(coreerrs.ErrUnavailable, "ensure retry queue schema: %w", err)
		}
	}
	if cfg.PurgeInterval > 0 {
		b.onShutdown(func(context.Context) error {
			store.StopPurge()
			return nil
		})
	}
	return store, nil
}

// onShutdown registers hook in the builder's scope, defaulting to the
// process-wide registry.
func (b *Builder[P]) onShutdown(hook coreruntime.ShutdownHook) {
	if b.shutdownHooks != nil {
		b.shutdownHooks.OnShutdown(hook)
		return
	}
	coreruntime.OnShutdown(hook)
}
