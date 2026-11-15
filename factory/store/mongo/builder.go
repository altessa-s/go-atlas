// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/store/mongo"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	memorystorage "github.com/altessa-s/go-atlas/store/mongo/storages/memory"
	schedtask "github.com/altessa-s/go-horai/task"
)

// StorageFunc creates the cursor storage of one storage type from the storage
// backend configuration, the builder's entry TTL (zero when WithTTL was not
// called; the NATS storage rejects it) and its logger.
type StorageFunc func(ctx context.Context, cfg *storageconfig.BackendConfig, ttl time.Duration, logger *slog.Logger) (mongo.Storage, error)

// CursorStorageBuilder assembles a [mongo.Storage] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [CursorStorageBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory storage. The Redis and NATS ones
// register their [StorageFunc] with [CursorStorageBuilder.UseStorage] from
// store/mongo/storages/{redis,nats}/factory; a configured type without one
// fails at Build naming the package to add.
type CursorStorageBuilder struct {
	corefactory.Base
	cfg  *storageconfig.BackendConfig
	errs []error
	ttl  time.Duration

	// Dependencies
	scheduler schedtask.Registrar
	storages  map[storageconfig.BackendType]StorageFunc
}

// New creates a [CursorStorageBuilder] for the given storage backend config.
// Config can be nil — the error surfaces at [CursorStorageBuilder.Build] time.
func New(cfg *storageconfig.BackendConfig) *CursorStorageBuilder {
	return &CursorStorageBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the cursor storage. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *CursorStorageBuilder) Build(ctx context.Context) (mongo.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if b.cfg.Type == storageconfig.BackendTypeMemory {
		return b.createMemoryStorage(), nil //nolint:contextcheck // memory storage handles its own context
	}
	if build, ok := b.storages[b.cfg.Type]; ok {
		return build(ctx, b.cfg, b.ttl, b.Logger())
	}
	switch b.cfg.Type {
	case storageconfig.BackendTypeRedis:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "cursor storage %q is not registered: add factory/store/mongo/storages/redis "+
			"with UseStorage(%q, redisfactory.Builder(client))", b.cfg.Type, b.cfg.Type)
	case storageconfig.BackendTypeNATS:
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "cursor storage %q is not registered: add factory/store/mongo/storages/nats "+
			"with UseStorage(%q, natsfactory.Builder(js))", b.cfg.Type, b.cfg.Type)
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", b.cfg.Type)
	}
}

// createMemoryStorage creates an in-memory cursor storage from configuration.
func (b *CursorStorageBuilder) createMemoryStorage() *memorystorage.Storage {
	opts := []memorystorage.Option{
		memorystorage.WithScheduler(b.scheduler),
	}
	opts = coreslices.AppendIfFunc(opts, b.cfg.Memory != nil && b.cfg.Memory.CleanupSchedule != "", func() []memorystorage.Option {
		return []memorystorage.Option{memorystorage.WithCleanupSchedule(b.cfg.Memory.CleanupSchedule)}
	})
	return memorystorage.New(b.ttl, opts...)
}
