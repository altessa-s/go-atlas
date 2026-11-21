// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/sqldb"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	cfg "github.com/altessa-s/go-atlas/config/requestidstore"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// StorageFunc creates a configured backend using caller-owned dependencies.
type StorageFunc func(context.Context, *cfg.Config) (requestid.Store, error)

// StoreBuilder assembles the selected storage and registers its cleanup hook.
type StoreBuilder struct {
	corefactory.Base
	config *cfg.Config
	stores map[cfg.StorageType]StorageFunc
	db     *sql.DB
	hooks  *coreruntime.HookGroup
}

// New constructs a builder for config.
func New(config *cfg.Config) *StoreBuilder {
	return &StoreBuilder{Base: corefactory.NewBase(nil), config: config, stores: make(map[cfg.StorageType]StorageFunc)}
}

// UseStorage registers a network backend supplied by its backend factory.
func (b *StoreBuilder) UseStorage(t cfg.StorageType, build StorageFunc) *StoreBuilder {
	b.stores[t] = build
	return b
}

// UseSQLDB injects the database for the built-in SQL backend.
func (b *StoreBuilder) UseSQLDB(db *sql.DB) *StoreBuilder {
	b.db = db
	return b
}

// UseShutdownHooks registers cleanup in a scoped group. The default is the process-wide registry.
func (b *StoreBuilder) UseShutdownHooks(h *coreruntime.HookGroup) *StoreBuilder {
	b.hooks = h
	return b
}

// Build validates the selected configuration and constructs its store.
func (b *StoreBuilder) Build(ctx context.Context) (requestid.Store, error) {
	if b.config == nil {
		return nil, corefactory.ErrConfigRequired
	}
	if err := b.config.Validate(); err != nil {
		return nil, err
	}
	var store requestid.Store
	var err error
	if b.config.Type == cfg.StorageTypeSQL {
		store, err = b.buildSQL(ctx)
	} else {
		build := b.stores[b.config.Type]
		if build == nil {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "request ID storage %q requires a backend factory", b.config.Type)
		}
		store, err = build(ctx, b.config)
	}
	if err != nil {
		return nil, b.WrapError(err, "build request ID storage")
	}
	if err := b.RequireDependency(store, "request ID storage"); err != nil {
		return nil, err
	}
	if closer, ok := store.(interface{ Close(context.Context) error }); ok {
		if b.hooks != nil {
			b.hooks.OnShutdown(closer.Close)
		} else {
			coreruntime.OnShutdown(closer.Close)
		}
	}
	return store, nil
}

func (b *StoreBuilder) buildSQL(ctx context.Context) (requestid.Store, error) {
	if err := b.RequireDependency(b.db, "SQL database"); err != nil {
		return nil, err
	}
	c := b.config.SQL
	store, err := sqldb.New(b.db, sqldb.Dialect(c.Dialect), sqldb.WithTableName(c.Table), sqldb.WithPurgeInterval(c.PurgeInterval),
		sqldb.WithPurgeBatchSize(c.PurgeBatchSize), sqldb.WithLogger(b.Logger()))
	if err != nil {
		return nil, err
	}
	if c.EnsureSchema {
		if err := store.EnsureSchema(ctx); err != nil {
			return nil, err
		}
	}
	return store, nil
}
