// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *StorageBuilder) UseLogger(v *slog.Logger) *StorageBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *StorageBuilder) UseDefaultLogger() *StorageBuilder {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one cache storage type
// (storageconfig.CacheStorageTypeRedis). A later registration of the same
// type replaces the earlier one; the memory storage is built in.
func (b *StorageBuilder) UseStorage(typ storageconfig.CacheStorageType, build StorageFunc) *StorageBuilder {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[storageconfig.CacheStorageType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}
