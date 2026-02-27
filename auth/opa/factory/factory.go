// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// Factory creates OPA managers and evaluators.
type Factory struct {
	corefactory.Base
	opts *options
}

// New creates a new OPA Factory.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base: corefactory.NewBase(cfg.logger),
		opts: cfg,
	}
}

// CreateManagerFromConfig creates an OPA Manager from configuration.
// It automatically creates the appropriate PolicySource based on the configuration.
func (f *Factory) CreateManagerFromConfig(ctx context.Context, cfg *config.OPA) (*opa.Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !cfg.IsEnabled() {
		return nil, f.WrapError(fmt.Errorf("OPA not enabled"), "configuration check failed")
	}

	// Create filesystem source
	fsOpts := []filesystem.Option{
		filesystem.WithLogger(f.opts.logger),
		filesystem.WithExtensions(cfg.FileExtensions...),
	}
	fsOpts = slices.AppendIf(fsOpts, cfg.IncludeData, filesystem.WithIncludeData())

	source, err := filesystem.New(cfg.BundlePath, fsOpts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to create policy source")
	}

	// Create manager options
	managerOpts := []opa.Option{
		opa.WithLogger(f.opts.logger),
		opa.WithHealthCoordinator(f.opts.healthCoordinator),
	}
	managerOpts = slices.AppendIf(managerOpts, cfg.DecisionLogging, opa.WithDecisionLogging())

	if f.opts.scheduler != nil && cfg.UpdateSchedule != "" {
		managerOpts = append(managerOpts,
			opa.WithScheduler(f.opts.scheduler),
			opa.WithUpdateSchedule(cfg.UpdateSchedule, cfg.RunOnStart),
		)
	}

	// Create manager
	manager, err := opa.NewManager(ctx, source, cfg.Query, managerOpts...)
	if err != nil {
		_ = source.Close()
		return nil, f.WrapError(err, "failed to create manager")
	}

	// Start watching if enabled
	if cfg.WatchBundle {
		if err = manager.StartWatching(ctx); err != nil {
			_ = manager.Close()
			return nil, f.WrapError(err, "failed to start watching")
		}
	}

	return manager, nil
}

// CreateSourceFromConfig creates a PolicySource from configuration.
// This is useful when you need more control over the manager creation.
func (f *Factory) CreateSourceFromConfig(cfg *config.OPA) (opa.PolicySource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	fsOpts := []filesystem.Option{
		filesystem.WithLogger(f.opts.logger),
		filesystem.WithExtensions(cfg.FileExtensions...),
	}
	fsOpts = slices.AppendIf(fsOpts, cfg.IncludeData, filesystem.WithIncludeData())

	return filesystem.New(cfg.BundlePath, fsOpts...)
}
