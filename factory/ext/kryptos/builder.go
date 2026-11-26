// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/altessa-s/go-horai/task"
	"github.com/altessa-s/go-kryptos/manager"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/retry"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/observability/health"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	kryptos "github.com/altessa-s/go-kryptos"
	memoryprovider "github.com/altessa-s/go-kryptos/providers/memory"
)

// UpdateTaskID is the ID of the scheduler task that runs the Manager's update
// cycle.
const UpdateTaskID = "secrets-cache-sync"

// HealthServiceName is the service name the Manager registers with the health
// coordinator.
const HealthServiceName = "secrets"

// ErrProviderConfigMissing is returned when the selected provider's own
// section (gcp, lockbox) is absent from the secrets configuration, which a
// caller that loads it without running its Validate can produce.
var ErrProviderConfigMissing = errors.New("secrets provider configuration is missing")

// ProviderBuilder creates the configured provider for a manager of T secrets.
type ProviderBuilder[T any] func(ctx context.Context, cfg *secretsconfig.Config) (kryptos.Provider[T], error)

// UpdateScheduler registers and removes the factory's periodic secrets task.
type UpdateScheduler interface {
	task.Registrar
	Unregister(context.Context, string) error
}

// ManagerBuilder assembles a [manager.Manager] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ManagerBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the memory provider: register the remote ones per
// provider type with [ManagerBuilder.UseProvider], for example the Vault one
// from factory/ext/kryptos/vault.
type ManagerBuilder[T any] struct {
	corefactory.Base
	cfg  *secretsconfig.Config
	errs []error

	// Dependencies
	scheduler         UpdateScheduler
	healthCoordinator health.ScopedReporter
	providers         map[secretsconfig.Provider]ProviderBuilder[T]
}

// New creates a [ManagerBuilder] for the given secrets config.
// Config can be nil — the error surfaces at [ManagerBuilder.Build] time.
func New[T any](cfg *secretsconfig.Config) *ManagerBuilder[T] {
	return &ManagerBuilder[T]{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// --- Build ---

// Build assembles the secrets Manager. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ManagerBuilder[T]) Build(ctx context.Context) (*manager.Manager[T], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(b.cfg, "secrets configuration"); err != nil {
		return nil, err
	}

	// Create provider based on configuration
	provider, err := createProvider[T](ctx, b)
	if err != nil {
		return nil, err
	}

	return newManager(ctx, b, provider)
}

// newManager builds the Manager that owns provider. Until it is built, the
// provider (its clients and connections) belongs to the builder, so every
// error path closes it; afterwards Manager.Close does. A Manager whose update
// task the scheduler rejects is closed, which closes the provider too.
func newManager[T any](ctx context.Context, b *ManagerBuilder[T], provider kryptos.Provider[T]) (*manager.Manager[T], error) {
	owned := &managedProvider[T]{Provider: provider}
	mgr, err := manager.New[T](owned, managerOptions[T](b)...)
	if err != nil {
		closeErr := provider.Close(context.WithoutCancel(ctx))
		return nil, errors.Join(coreerrs.WrapOperation(err, "create secrets manager"), closeErr)
	}

	if err := registerUpdateTask(ctx, b, mgr, owned); err != nil {
		return nil, errors.Join(err, mgr.Close(context.WithoutCancel(ctx)))
	}

	if !nilcheck.IsNil(b.healthCoordinator) {
		_, owned.unregisterHealth = b.healthCoordinator.RegisterScopedService(HealthServiceName, healthChecker(mgr))
	}

	return mgr, nil
}

// The manager closes its provider after draining in-flight work. Bind cleanup
// there so callers still receive the manager and use its normal Close method.
type managedProvider[T any] struct {
	kryptos.Provider[T]
	unregisterHealth func()
	unregisterTask   func(context.Context) error
}

func (p *managedProvider[T]) Close(ctx context.Context) error {
	if p.unregisterHealth != nil {
		p.unregisterHealth()
	}
	if p.unregisterTask == nil {
		return p.Provider.Close(ctx)
	}

	const unregisterTimeout = 5 * time.Second
	cleanupCtx, cancel := corectx.WithMaxTimeout(context.WithoutCancel(ctx), unregisterTimeout)
	defer cancel()
	taskErr := p.unregisterTask(cleanupCtx)
	return errors.Join(taskErr, p.Provider.Close(ctx))
}

// registerUpdateTask schedules the Manager's update cycle when a scheduler is
// set and the configuration has an update schedule. The memory provider is
// the only source of its values, so it is never reconciled.
func registerUpdateTask[T any](ctx context.Context, b *ManagerBuilder[T], mgr *manager.Manager[T], owner *managedProvider[T]) error {
	scheduler := b.scheduler
	if nilcheck.IsNil(scheduler) || b.cfg.UpdateSchedule == "" || b.cfg.Provider == secretsconfig.ProviderMemory {
		return nil
	}
	err := scheduler.Register(ctx, task.Config{
		ID:          UpdateTaskID,
		Description: "Synchronize secrets cache with storage",
		Func:        mgr.RunUpdateCycle,
		Schedule:    b.cfg.UpdateSchedule,
		RunOnStart:  b.cfg.RunOnStart,
		Priority:    task.PriorityNormal,
	})
	if err != nil {
		return coreerrs.WrapOperation(err, "register secrets update cycle task")
	}
	owner.unregisterTask = func(ctx context.Context) error {
		if err := scheduler.Unregister(ctx, UpdateTaskID); err != nil {
			return diagnostics.ClassifiedError("unregister secrets update cycle task", err)
		}
		return nil
	}
	return nil
}

// healthChecker adapts the Manager's health to the health system.
func healthChecker[T any](mgr *manager.Manager[T]) health.Checker {
	return health.Func(func(ctx context.Context) health.ServingStatus {
		switch mgr.Health(ctx) {
		case kryptos.Serving:
			return health.StatusServing
		case kryptos.Degraded:
			return health.StatusDegraded
		default:
			return health.StatusNotServing
		}
	})
}

// createMemoryProvider creates an in-memory provider serving the configured
// values. They are strings, so T must accept a string (string or any).
func createMemoryProvider[T any](cfg *secretsconfig.Memory) (*memoryprovider.Store[T], error) {
	var values map[string]T
	if cfg != nil && len(cfg.Values) > 0 {
		values = make(map[string]T, len(cfg.Values))
		for key, v := range cfg.Values {
			typed, ok := any(v.Expose()).(T)
			if !ok {
				return nil, fmt.Errorf("memory secrets provider: values are strings, the manager holds %T", *new(T))
			}
			values[key] = typed
		}
	}
	return memoryprovider.New[T](values)
}

// createProvider creates the provider of T secrets selected by the
// configuration: the memory one, or the one registered for its type.
func createProvider[T any](ctx context.Context, b *ManagerBuilder[T]) (kryptos.Provider[T], error) {
	if b.cfg.Provider == secretsconfig.ProviderMemory {
		return createMemoryProvider[T](b.cfg.Memory)
	}
	build, ok := b.providers[b.cfg.Provider]
	if !ok {
		if b.cfg.Provider == secretsconfig.ProviderVault || b.cfg.Provider == secretsconfig.ProviderGCP ||
			b.cfg.Provider == secretsconfig.ProviderLockbox {
			return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "secrets provider %q is not registered: add factory/ext/kryptos/%s "+
				"with UseProvider(%q, %sfactory.Builder(...))", b.cfg.Provider, b.cfg.Provider, b.cfg.Provider, b.cfg.Provider)
		}
		return nil, coreerrs.Wrapf(corefactory.Errorf(corefactory.ErrUnsupported, "unsupported provider: %s", b.cfg.Provider),
			"invalid configuration for %s", "secrets provider")
	}
	return build(ctx, b.cfg)
}

// managerOptions builds manager options from configuration for a Manager of T
// secrets.
//
// cache.maxSize zero keeps the Manager's default size and cache.shardCount
// zero its single shard.
func managerOptions[T any](b *ManagerBuilder[T]) []manager.Option[T] {
	opts := []manager.Option[T]{
		manager.WithLogger[T](b.Logger()),
		manager.WithMaxRetries[T](b.cfg.Retry.MaxAttempts),
		manager.WithBackoff[T](retry.Exponential(
			b.cfg.Retry.BaseDelay,
			retry.WithExponentialMaxDelay(b.cfg.Retry.MaxDelay),
			retry.WithExponentialFactor(b.cfg.Retry.Multiplier),
			retry.WithExponentialJitter(b.cfg.Retry.Jitter),
		)),
	}
	opts = slices.AppendIf(opts, b.cfg.EmptyListingThreshold > 0, manager.WithEmptyListingThreshold[T](b.cfg.EmptyListingThreshold))
	opts = slices.AppendIf(opts, b.cfg.AllowShallowClone, manager.WithAllowShallowClone[T]())
	opts = slices.AppendIf(opts, b.cfg.Cache.MaxSize > 0, manager.WithCacheSize[T](b.cfg.Cache.MaxSize))
	return slices.AppendIf(opts, b.cfg.Cache.ShardCount > 0, manager.WithCacheShards[T](b.cfg.Cache.ShardCount))
}
