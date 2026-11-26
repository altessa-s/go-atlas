// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	memorystorage "github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

// BudgetLimiterBuilder assembles a [budget.Limiter] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [BudgetLimiterBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory storage. The Redis one registers its
// [storagesfactory.StorageFunc] with [BudgetLimiterBuilder.UseStorage] from
// factory/coord/limiters/storages/redis; a configured type without one fails at
// Build naming the package to add.
type BudgetLimiterBuilder struct {
	corefactory.Base
	cfg  *limiterconfig.Budget
	errs []error

	// Dependencies
	scheduler schedtask.Registrar
	collector metrics.Meter

	storages map[limiterconfig.StorageType]storagesfactory.StorageFunc
}

// New creates a [BudgetLimiterBuilder] for the given budget limiter config.
// Config can be nil — the error surfaces at [BudgetLimiterBuilder.Build] time.
func New(cfg *limiterconfig.Budget) *BudgetLimiterBuilder {
	return &BudgetLimiterBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the budget limiter. It creates the storage from config internally,
// then creates the limiter. Errors from fluent methods are accumulated
// and reported here via [errors.Join]. ctx bounds storage provisioning.
func (b *BudgetLimiterBuilder) Build(ctx context.Context) (*budget.Limiter, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := b.createStorage(ctx)
	if err != nil {
		return nil, err
	}

	return budget.New(b.cfg.Limit, b.cfg.Period, storage, b.applyDefaults()...)
}

// applyDefaults returns factory defaults as limiter options.
func (b *BudgetLimiterBuilder) applyDefaults() []budget.Option {
	return []budget.Option{
		budget.WithMeter(b.collector),
	}
}

// createStorage creates a storage backend based on configuration.
func (b *BudgetLimiterBuilder) createStorage(ctx context.Context) (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "storage configuration is required")
	}

	if b.cfg.Storage.Type == limiterconfig.StorageTypeMemory {
		return b.createMemoryStorage() //nolint:contextcheck // the memory cleanup task is process-scoped, not tied to the startup ctx
	}
	return storagesfactory.BuildRegistered(ctx, b.cfg.Storage, b.storages, "budget")
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *BudgetLimiterBuilder) createMemoryStorage() (*memorystorage.Provider, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "memory storage configuration is required")
	}

	opts := []memorystorage.Option{
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	}
	opts = coreslices.AppendIf(opts, b.cfg.InsecureEvictLRU, memorystorage.WithInsecureEvictLRU())
	return memorystorage.New(opts...), nil
}
