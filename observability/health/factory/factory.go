// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// Factory creates [health.Coordinator] instances from configuration.
// Safe for concurrent use after construction.
type Factory struct {
	corefactory.Base
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base: corefactory.NewBase(cfg.logger),
	}
}

// CreateCoordinatorFromConfig creates a [health.Coordinator] from the given [config.Health].
// Additional opts override config-derived values.
func (f *Factory) CreateCoordinatorFromConfig(cfg *config.Health, opts ...health.Option) (*health.Coordinator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	configOpts := f.configToOptions(cfg)
	// Combine config options with user options
	opts = append(configOpts, opts...)
	// Prepend logger from factory
	opts = append([]health.Option{health.WithLogger(f.Logger())}, opts...)

	return health.New(opts...), nil
}

// configToOptions converts configuration settings to a slice of health options.
func (f *Factory) configToOptions(cfg *config.Health) []health.Option {
	return []health.Option{
		health.WithWatcherChannelBuffer(cfg.WatcherChannelBuffer),
		health.WithMaxWatchersPerService(cfg.MaxWatchersPerService),
		health.WithNumShards(cfg.NumShards),
		health.WithMaxConcurrentHealthChecks(cfg.MaxConcurrentHealthChecks),
		health.WithStatusCacheTTL(cfg.StatusCacheTTL),
		health.WithCheckTimeout(cfg.CheckTimeout),
		health.WithAdaptiveBufferThreshold(cfg.AdaptiveBufferThreshold),
		health.WithAdaptiveBufferMultiplier(cfg.AdaptiveBufferMultiplier),
		health.WithMaxAdaptiveBuffer(cfg.MaxAdaptiveBuffer),
	}
}
