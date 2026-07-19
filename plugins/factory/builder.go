// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/plugins"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// ManagerBuilder assembles a [plugins.Manager] step by step using a fluent
// API. Create instances with [NewManager]. The builder is not safe for
// concurrent use.
type ManagerBuilder struct {
	corefactory.Base
	cfg *config.Plugins

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

// UseHealthCoordinator registers the manager with a health coordinator
// under the default service name "plugins". Use
// [ManagerBuilder.UseHealthServiceName] to override the name.
func (b *ManagerBuilder) UseHealthCoordinator(hc *health.Coordinator) *ManagerBuilder {
	b.healthCoordinator = hc
	return b
}

// UseHealthServiceName overrides the service name under which the manager
// is registered with the health coordinator. The default is "plugins".
// An empty value is ignored.
func (b *ManagerBuilder) UseHealthServiceName(name string) *ManagerBuilder {
	if name != "" {
		b.healthServiceName = name
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
//
// On any post-construction failure (Load failure, watcher start failure)
// the partially-built manager is closed before the error is returned, so
// the caller does not leak the manager's goroutines or watcher state.
func (b *ManagerBuilder) Build(ctx context.Context) (*plugins.Manager, error) {
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

	// Auto-wire the host service version from appinfo when it has been
	// stamped via -ldflags. The default "0.0.0" is treated as "unset" so
	// dev builds (go run …) do not fail to load plugins that declare a
	// real HostVersion. Production builds with a real version get the
	// HostVersionEnforce check active without further configuration.
	opts = coreslices.AppendIf(opts,
		appinfo.Version != "" && appinfo.Version != "0.0.0",
		plugins.WithHostVersion(appinfo.Version),
	)

	opts = coreslices.AppendIf(opts, b.cfg.Signature.Mode != "", plugins.WithSignature(
		plugins.SignatureOptionsFromConfig(b.cfg.Signature.Mode, b.cfg.Signature.PublicKeyPath),
	))

	opts = coreslices.AppendIf(opts, len(b.cfg.Load) > 0, plugins.WithLoad(b.cfg.Load...))
	opts = coreslices.AppendIf(opts, len(b.cfg.Disabled) > 0, plugins.WithDisabled(b.cfg.Disabled...))

	mgr := plugins.NewManager(opts...)

	if err := mgr.Load(ctx); err != nil {
		// A partially-loaded manager may already hold sandbox state
		// and ready plugins. Close it to release goroutines and to
		// ensure the registry is reset before the caller drops the
		// reference.
		_ = mgr.Close()
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
