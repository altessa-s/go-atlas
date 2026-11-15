// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/cache/storages"

	freecacheprovider "github.com/altessa-s/go-atlas/cache/storages/freecache"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// StorageFunc creates the cache storage of one storage type from the cache
// storage configuration.
type StorageFunc func(cfg *storageconfig.CacheStorageConfig) (storages.Storage, error)

// StorageBuilder assembles a cache [storages.Storage] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [StorageBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory FreeCache storage. The Redis one
// registers its [StorageFunc] with [StorageBuilder.UseStorage] from
// factory/cache/storages/redis; a configured type without one fails at
// Build naming the package to add.
type StorageBuilder struct {
	corefactory.Base
	cfg  *storageconfig.CacheStorageConfig
	errs []error

	storages map[storageconfig.CacheStorageType]StorageFunc
}

// New creates a [StorageBuilder] for the given cache storage config.
// Config can be nil — the builder returns a FreeCache (memory) provider if nil.
func New(cfg *storageconfig.CacheStorageConfig) *StorageBuilder {
	return &StorageBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the cache provider. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *StorageBuilder) Build() (storages.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil || b.cfg.Type == storageconfig.CacheStorageTypeMemory {
		return freecacheprovider.New(), nil
	}
	if build, ok := b.storages[b.cfg.Type]; ok {
		return build(b.cfg)
	}
	if b.cfg.Type == storageconfig.CacheStorageTypeRedis {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "cache storage %q is not registered: add factory/cache/storages/redis "+
			"with UseStorage(%q, redisfactory.Builder(client))", b.cfg.Type, b.cfg.Type)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", b.cfg.Type)
}
