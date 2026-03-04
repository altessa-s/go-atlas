// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
	"github.com/altessa-s/go-atlas/auth/opa/sources/gitlab"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// ManagerBuilder assembles an [opa.Manager] from configuration and
// injected dependencies using a fluent API with deferred error accumulation.
type ManagerBuilder struct {
	corefactory.Base
	cfg  *config.OPA
	errs []error

	// Dependencies (set via Use*).
	scheduler         corescheduler.TaskRegistrar
	healthCoordinator *health.Coordinator
}

// New creates a new [ManagerBuilder] for the given OPA config.
// A nil cfg is accepted; the error surfaces at [ManagerBuilder.Build] time.
func New(cfg *config.OPA) *ManagerBuilder {
	return &ManagerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the OPA manager. All errors accumulated
// during the fluent chain are returned here.
func (b *ManagerBuilder) Build(ctx context.Context) (*opa.Manager, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !b.cfg.IsEnabled() {
		return nil, b.WrapError(fmt.Errorf("OPA not enabled"), "configuration check failed")
	}

	source, err := b.buildSource()
	if err != nil {
		return nil, err
	}

	manager, err := b.buildManager(ctx, source)
	if err != nil {
		_ = source.Close()
		return nil, err
	}

	if b.cfg.WatchBundle {
		if err = manager.StartWatching(ctx); err != nil {
			_ = manager.Close()
			return nil, b.WrapError(err, "failed to start watching")
		}
	}

	return manager, nil
}

// buildSource creates a PolicySource from config based on the configured provider.
func (b *ManagerBuilder) buildSource() (opa.PolicySource, error) {
	switch b.cfg.Source {
	case config.OPASourceGitLab:
		return b.buildGitLabSource()
	default:
		return b.buildFilesystemSource()
	}
}

// buildFilesystemSource creates a filesystem PolicySource from config.
func (b *ManagerBuilder) buildFilesystemSource() (opa.PolicySource, error) {
	cfg := b.cfg

	fsOpts := []filesystem.Option{
		filesystem.WithLogger(b.Logger()),
		filesystem.WithExtensions(cfg.FileExtensions...),
	}
	fsOpts = slices.AppendIf(fsOpts, cfg.IncludeData, filesystem.WithIncludeData())

	source, err := filesystem.New(cfg.BundlePath, fsOpts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create filesystem policy source")
	}
	return source, nil
}

// buildGitLabSource creates a GitLab PolicySource from config.
func (b *ManagerBuilder) buildGitLabSource() (opa.PolicySource, error) {
	gl := b.cfg.GitLab

	opts := []gitlab.Option{
		gitlab.WithEndpoint(gl.Endpoint),
		gitlab.WithToken(gl.Token.Expose()),
		gitlab.WithProjectID(gl.ProjectID),
		gitlab.WithLogger(b.Logger()),
	}

	if gl.Ref != "" {
		opts = append(opts, gitlab.WithRef(gl.Ref))
	}

	if gl.Dir != "" {
		opts = append(opts, gitlab.WithDir(gl.Dir))
	}

	if gl.RetryMax > 0 {
		opts = append(opts, gitlab.WithRetryMax(gl.RetryMax))
	}

	if gl.RetryWaitMin > 0 {
		opts = append(opts, gitlab.WithRetryWaitMin(gl.RetryWaitMin))
	}

	if gl.RetryWaitMax > 0 {
		opts = append(opts, gitlab.WithRetryWaitMax(gl.RetryWaitMax))
	}

	opts = slices.AppendIf(opts, b.cfg.IncludeData, gitlab.WithIncludeData())

	source, err := gitlab.New(opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create gitlab policy source")
	}
	return source, nil
}

// buildManager creates the OPA Manager from source and config.
func (b *ManagerBuilder) buildManager(ctx context.Context, source opa.PolicySource) (*opa.Manager, error) {
	cfg := b.cfg

	managerOpts := []opa.Option{
		opa.WithLogger(b.Logger()),
		opa.WithHealthCoordinator(b.healthCoordinator),
	}
	managerOpts = slices.AppendIf(managerOpts, cfg.DecisionLogging, opa.WithDecisionLogging())

	if cfg.PollInterval > 0 {
		managerOpts = append(managerOpts, opa.WithPollInterval(cfg.PollInterval))
	}

	if b.scheduler != nil && cfg.UpdateSchedule != "" {
		managerOpts = append(managerOpts,
			opa.WithScheduler(b.scheduler),
			opa.WithUpdateSchedule(cfg.UpdateSchedule, cfg.RunOnStart),
		)
	}

	manager, err := opa.NewManager(ctx, source, cfg.Query, managerOpts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create manager")
	}
	return manager, nil
}
