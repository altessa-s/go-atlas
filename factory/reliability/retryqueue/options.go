// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"log/slog"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// UseLogger sets the logger for the builder and the sqldb store's background
// purge.
func (b *Builder[P]) UseLogger(v *slog.Logger) *Builder[P] {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder[P]) UseDefaultLogger() *Builder[P] {
	return b.UseLogger(slog.Default())
}

// UseStorage registers a driver or custom storage function. A later registration of the same type replaces the earlier one; the memory
// and sqldb stores are built in.
func (b *Builder[P]) UseStorage(typ retryqueueconfig.StorageType, build StorageFunc[P]) *Builder[P] {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseStorage %q: nil builder", typ))
		return b
	}
	if typ == retryqueueconfig.StorageTypeMemory || typ == retryqueueconfig.StorageTypeSQL || typ == "" {
		b.errs = append(b.errs, corefactory.Errorf(corefactory.ErrUnsupported,
			"UseStorage %q: expected a nonempty type other than built-in memory or sqldb", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[retryqueueconfig.StorageType]StorageFunc[P]{}
	}
	b.storages[typ] = build
	return b
}

// UseSQLDB injects the database handle used when the storage type is "sqldb".
// The caller owns the handle and registers the driver.
func (b *Builder[P]) UseSQLDB(v *sql.DB) *Builder[P] {
	return b.UseSQLPool(v, nil)
}

// UseSQLPool injects a database and its per-call transaction resolver.
func (b *Builder[P]) UseSQLPool(db *sql.DB, resolve func(context.Context) (*sql.Tx, bool)) *Builder[P] {
	b.sqlDB, b.resolveTx = db, resolve
	return b
}

// UseMemoryPayloadClone supplies the memory store's payload snapshot function.
// Nil uses the memory store's default cloning policy.
func (b *Builder[P]) UseMemoryPayloadClone(clone func(P) P) *Builder[P] {
	b.clonePayload = clone
	return b
}

// UseCodec sets the codec the sqldb store encodes payloads with (default
// JSON). The MongoDB store encodes payloads as BSON and ignores it.
func (b *Builder[P]) UseCodec(v serializer.Serializer) *Builder[P] {
	b.codec = v
	return b
}

// UseShutdownHooks sets the scope the builder registers the components it owns
// into — the sqldb store's StopPurge, which stops its background purge. Without it
// they go into the process-wide registry, which runs once for the whole
// program; pass a group when the queue must be stoppable on its own.
func (b *Builder[P]) UseShutdownHooks(v *coreruntime.HookGroup) *Builder[P] {
	b.shutdownHooks = v
	return b
}

// RegisteredStorageTypes returns a snapshot of the names registered with UseStorage for standalone configuration validation.
func (b *Builder[P]) RegisteredStorageTypes() []retryqueueconfig.StorageType {
	return slices.Sorted(maps.Keys(b.storages))
}
