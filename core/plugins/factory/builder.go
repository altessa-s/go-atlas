// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/plugins"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// ManagerBuilder assembles a [plugins.Manager] step by step using a fluent API.
// Create instances with [NewManager]. Errors are accumulated and reported at
// [ManagerBuilder.Build] time. The builder is not safe for concurrent use.
type ManagerBuilder struct {
	corefactory.Base
	cfg  *config.Plugins
	errs []error

	healthCoordinator *health.Coordinator
	healthServiceName string
}

// NewManager creates a [ManagerBuilder] for the given plugins config.
// Config can be nil — the error surfaces at [ManagerBuilder.Build] time.
func NewManager(cfg *config.Plugins) *ManagerBuilder {
	return &ManagerBuilder{
		Base:              corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:               cfg,
		healthServiceName: "plugins",
	}
}

// UseLogger sets the logger for the manager.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseHealthCoordinator registers the manager with a health coordinator.
func (b *ManagerBuilder) UseHealthCoordinator(hc *health.Coordinator, serviceName ...string) *ManagerBuilder {
	b.healthCoordinator = hc
	if len(serviceName) > 0 && serviceName[0] != "" {
		b.healthServiceName = serviceName[0]
	}
	return b
}

// Build assembles the plugin manager, loads plugins from the configured
// directory, and returns the ready manager.
//
// Returns an error if the configuration is nil or if [config.Plugins.IsEnabled]
// reports false. Callers that tolerate a disabled plugin manager should
// check [config.Plugins.IsEnabled] before invoking Build, matching the
// pattern used by other optional subsystem factories (auth/opa, secrets).
func (b *ManagerBuilder) Build(ctx context.Context) (*plugins.Manager, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(b.cfg, "plugins configuration"); err != nil {
		return nil, err
	}

	if !b.cfg.IsEnabled() {
		return nil, b.Errorf("plugin manager is not enabled")
	}

	opts := []plugins.Option{
		plugins.WithLogger(b.Logger()),
		plugins.WithDir(b.cfg.Dir),
		plugins.WithInitTimeout(b.cfg.InitTimeout),
		plugins.WithWatchDebounce(b.cfg.WatchDebounce),
		plugins.WithSandbox(plugins.SandboxOptionsFromConfig(b.cfg.Sandbox)),
	}

	if len(b.cfg.Load) > 0 {
		opts = append(opts, plugins.WithLoad(b.cfg.Load...))
	}
	if len(b.cfg.Disabled) > 0 {
		opts = append(opts, plugins.WithDisabled(b.cfg.Disabled...))
	}

	mgr := plugins.NewManager(opts...)

	if err := mgr.Load(ctx); err != nil {
		return nil, b.WrapError(err, "failed to load plugins")
	}

	// Start the filesystem watcher AFTER Load so the initial plugin set is
	// registered before any fsnotify-driven Reload races with it. ctx must
	// be the application context; when it is canceled the watcher exits.
	if b.cfg.Watch {
		if err := mgr.StartWatching(ctx); err != nil {
			_ = mgr.Close()
			return nil, b.WrapError(err, "failed to start plugin watcher")
		}
	}

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(b.healthServiceName, mgr)
	}

	return mgr, nil
}
