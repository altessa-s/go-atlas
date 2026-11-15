// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"time"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *CursorStorageBuilder) UseLogger(v *slog.Logger) *CursorStorageBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *CursorStorageBuilder) UseDefaultLogger() *CursorStorageBuilder {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one cursor storage type
// (storageconfig.BackendTypeRedis, storageconfig.BackendTypeNATS). A later
// registration of the same type replaces the earlier one; the memory storage
// is built in.
func (b *CursorStorageBuilder) UseStorage(typ storageconfig.BackendType, build StorageFunc) *CursorStorageBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[storageconfig.BackendType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *CursorStorageBuilder) UseScheduler(v schedtask.Registrar) *CursorStorageBuilder {
	b.scheduler = v
	return b
}

// WithTTL sets the TTL for cursor storage entries.
func (b *CursorStorageBuilder) WithTTL(ttl time.Duration) *CursorStorageBuilder {
	b.ttl = ttl
	return b
}
