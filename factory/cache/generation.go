// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/cache/generation"

	memorygen "github.com/altessa-s/go-atlas/cache/generation/storages/memory"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// GenerationStoreFunc creates the generation store of one storage type from
// the cache storage configuration.
type GenerationStoreFunc func(cfg *storageconfig.CacheStorageConfig) (generation.Storage, error)

// GenerationStoreBuilder assembles a cache [generation.Storage] from the same
// [storageconfig.CacheStorageConfig] that selects the cache storage, so a
// service configures one `storage` block for both. Create instances with
// [NewGenerationStore]. The builder is not safe for concurrent use.
//
// The builder builds only the in-process store. The Redis one registers its
// [GenerationStoreFunc] with [GenerationStoreBuilder.UseStore] from
// factory/cache/generation/storages/redis.
type GenerationStoreBuilder struct {
	corefactory.Base
	cfg  *storageconfig.CacheStorageConfig
	errs []error

	stores map[storageconfig.CacheStorageType]GenerationStoreFunc
}

// NewGenerationStore creates a [GenerationStoreBuilder] for the given cache
// storage config. A nil config or type memory builds an in-process store
// (not shared between replicas); any other type uses the store registered
// for it.
func NewGenerationStore(cfg *storageconfig.CacheStorageConfig) *GenerationStoreBuilder {
	return &GenerationStoreBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// UseLogger sets the logger for the builder.
func (b *GenerationStoreBuilder) UseLogger(v *slog.Logger) *GenerationStoreBuilder {
	b.SetLogger(v)
	return b
}

// UseStore registers the generation store function of one storage type
// (storageconfig.CacheStorageTypeRedis). A later registration of the same
// type replaces the earlier one; the in-process store is built in.
func (b *GenerationStoreBuilder) UseStore(typ storageconfig.CacheStorageType, build GenerationStoreFunc) *GenerationStoreBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStore %q: nil function", typ))
		return b
	}
	if b.stores == nil {
		b.stores = map[storageconfig.CacheStorageType]GenerationStoreFunc{}
	}
	b.stores[typ] = build
	return b
}

// Build assembles the generation store.
func (b *GenerationStoreBuilder) Build() (generation.Storage, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil || b.cfg.Type == storageconfig.CacheStorageTypeMemory {
		return memorygen.New(), nil
	}
	if build, ok := b.stores[b.cfg.Type]; ok {
		return build(b.cfg)
	}
	if b.cfg.Type == storageconfig.CacheStorageTypeRedis {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "generation store %q is not registered: add factory/cache/generation/storages/redis "+
			"with UseStore(%q, redisgenfactory.Builder(client))", b.cfg.Type, b.cfg.Type)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported generation store type: %s", b.cfg.Type)
}
