// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas/observability/metrics"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder.
func (b *BudgetLimiterBuilder) UseLogger(v *slog.Logger) *BudgetLimiterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *BudgetLimiterBuilder) UseDefaultLogger() *BudgetLimiterBuilder {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one storage type
// (limiterconfig.StorageTypeRedis). A later registration of the same type
// replaces the earlier one; the memory storage is built in.
func (b *BudgetLimiterBuilder) UseStorage(typ limiterconfig.StorageType, build storagesfactory.StorageFunc) *BudgetLimiterBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[limiterconfig.StorageType]storagesfactory.StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *BudgetLimiterBuilder) UseScheduler(v schedtask.Registrar) *BudgetLimiterBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the [metrics.Meter] for recording budget limiter metrics.
func (b *BudgetLimiterBuilder) UseMeter(v metrics.Meter) *BudgetLimiterBuilder {
	b.collector = v
	return b
}

// RegisteredStorageTypes returns a snapshot of the names registered with UseStorage for standalone configuration validation.
func (b *BudgetLimiterBuilder) RegisteredStorageTypes() []limiterconfig.StorageType {
	return slices.Sorted(maps.Keys(b.storages))
}
