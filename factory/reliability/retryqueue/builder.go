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

	storages map[retryqueueconfig.StorageType]StorageFunc[P]
	sqlDB    *sql.DB
	codec    serializer.Serializer

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
// I/O (SQL schema or MongoDB index creation).
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
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate retry queue config: %w", err)
	}

	storage := cfg.Storage
	switch storage.Type {
	case retryqueueconfig.StorageTypeMemory:
		return memorystore.New[P](), nil
	case retryqueueconfig.StorageTypeSQL:
		if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
			return nil, err
		}
		return b.buildSQLStore(ctx, storage.SQL)
	case retryqueueconfig.StorageTypeMongo:
		if build, ok := b.storages[storage.Type]; ok {
			return build(ctx, storage)
		}
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "retry queue storage %q is not registered: add "+
			"factory/reliability/retryqueue/storages/mongo with UseStorage(%q, mongofactory.Builder[P](db))", storage.Type, storage.Type)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported retry queue storage type %q", storage.Type)
	}
}

// ensureSchemaTimeout bounds the schema creation buildSQLStore runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist. With a purge interval it registers
// the store's Close as a shutdown hook, which stops the background purge.
func (b *Builder[P]) buildSQLStore(ctx context.Context, cfg *retryqueueconfig.SQLStorageConfig) (*sqlstore.Store[P], error) {
	store, err := sqlstore.New[P](b.sqlDB, sqlstore.Dialect(cfg.Dialect),
		sqlstore.WithTableName(cfg.Table),
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
		schemaCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureSchemaTimeout)
		defer cancel()
		if err := store.EnsureSchema(schemaCtx); err != nil {
			return nil, b.WrapError(err, "ensure retry queue schema")
		}
	}
	if cfg.PurgeInterval > 0 {
		b.onShutdown(store.Close)
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
