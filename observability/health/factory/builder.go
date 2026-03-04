// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// CoordinatorBuilder assembles a [health.Coordinator] from configuration
// using a fluent API with deferred error accumulation.
type CoordinatorBuilder struct {
	corefactory.Base
	cfg  *config.Health
	errs []error
}

// New creates a new [CoordinatorBuilder] for the given health config.
func New(cfg *config.Health) *CoordinatorBuilder {
	return &CoordinatorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the health coordinator.
func (b *CoordinatorBuilder) Build() (*health.Coordinator, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := []health.Option{
		health.WithLogger(b.Logger()),
		health.WithWatcherChannelBuffer(b.cfg.WatcherChannelBuffer),
		health.WithMaxWatchersPerService(b.cfg.MaxWatchersPerService),
		health.WithNumShards(b.cfg.NumShards),
		health.WithMaxConcurrentHealthChecks(b.cfg.MaxConcurrentHealthChecks),
		health.WithStatusCacheTTL(b.cfg.StatusCacheTTL),
		health.WithCheckTimeout(b.cfg.CheckTimeout),
		health.WithAdaptiveBufferThreshold(b.cfg.AdaptiveBufferThreshold),
		health.WithAdaptiveBufferMultiplier(b.cfg.AdaptiveBufferMultiplier),
		health.WithMaxAdaptiveBuffer(b.cfg.MaxAdaptiveBuffer),
	}

	return health.New(opts...), nil
}
