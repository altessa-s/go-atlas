// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and the orchestrator it builds.
func (b *Builder[T]) UseLogger(v *slog.Logger) *Builder[T] {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *Builder[T]) UseDefaultLogger() *Builder[T] {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one saga storage type (nats,
// mongo or redis). A later registration of the same type replaces the earlier
// one; the memory and sqldb stores are built in.
func (b *Builder[T]) UseStorage(typ sagaconfig.StorageType, build StorageFunc) *Builder[T] {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[sagaconfig.StorageType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseSQLDB injects the database handle used when the storage type is "sqldb".
// The caller owns the handle and registers the driver.
func (b *Builder[T]) UseSQLDB(v *sql.DB) *Builder[T] {
	return b.UseSQLPool(v, nil)
}

// UseSQLPool injects a database and its per-call transaction resolver.
func (b *Builder[T]) UseSQLPool(db *sql.DB, resolve func(context.Context) (*sql.Tx, bool, error)) *Builder[T] {
	b.sqlDB, b.resolveTx = db, resolve
	return b
}

// UseMeter sets the Prometheus metrics collector for the orchestrator.
func (b *Builder[T]) UseMeter(v metrics.Meter) *Builder[T] {
	b.collector = v
	return b
}

// UseScheduler registers the background recovery cycle with the given task
// registrar.
func (b *Builder[T]) UseScheduler(v schedtask.Registrar) *Builder[T] {
	b.scheduler = v
	return b
}

// UseLeaderElector gates the recovery cycle so only the elected leader scans
// the store.
func (b *Builder[T]) UseLeaderElector(v saga.LeaderElector) *Builder[T] {
	b.leaderElector = v
	return b
}

// UseSerializer sets the codec used to (de)serialize the saga payload.
func (b *Builder[T]) UseSerializer(v serializer.Serializer) *Builder[T] {
	b.serializer = v
	return b
}

// UseOnDeadLetter registers a callback fired when a saga reaches the terminal
// Failed state.
func (b *Builder[T]) UseOnDeadLetter(v saga.DeadLetterFunc) *Builder[T] {
	b.onDeadLetter = v
	return b
}

// UseShouldRetry installs a predicate deciding whether a step or compensation
// error is retryable.
func (b *Builder[T]) UseShouldRetry(v func(error) bool) *Builder[T] {
	b.shouldRetry = v
	return b
}

// RegisteredStorageTypes returns a snapshot of the names registered with UseStorage for standalone configuration validation.
func (b *Builder[T]) RegisteredStorageTypes() []sagaconfig.StorageType {
	return slices.Sorted(maps.Keys(b.storages))
}
