// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/secrets"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	gcpprovider "github.com/altessa-s/go-atlas/security/secrets/providers/gcp"
	lockboxprovider "github.com/altessa-s/go-atlas/security/secrets/providers/lockbox"
	memoryprovider "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
	vaultprovider "github.com/altessa-s/go-atlas/security/secrets/providers/vault"
	vaultApi "github.com/hashicorp/vault/api"
)

// ManagerBuilder assembles a [secrets.Manager] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ManagerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ManagerBuilder struct {
	corefactory.Base
	cfg  *config.Secrets
	errs []error

	// Dependencies
	scheduler         corescheduler.TaskRegistrar
	healthCoordinator *health.Coordinator
	vaultClient       *vaultApi.Client
}

// New creates a [ManagerBuilder] for the given secrets config.
// Config can be nil — the error surfaces at [ManagerBuilder.Build] time.
func New(cfg *config.Secrets) *ManagerBuilder {
	return &ManagerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// --- Build ---

// Build assembles the secrets Manager. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ManagerBuilder) Build(ctx context.Context) (*secrets.Manager[any], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(b.cfg, "secrets configuration"); err != nil {
		return nil, err
	}

	// Create provider based on configuration
	provider, err := b.createProvider(ctx)
	if err != nil {
		return nil, err
	}

	// Build manager options from config
	managerOpts, err := b.buildManagerOptions()
	if err != nil {
		return nil, err
	}

	// Add scheduler and update schedule options if scheduler is available
	managerOpts = slices.AppendIf(managerOpts, b.scheduler != nil,
		secrets.WithScheduler(b.scheduler),
		secrets.WithUpdateSchedule(b.cfg.UpdateSchedule, b.cfg.RunOnStart),
	)

	managerOpts = slices.AppendIf(managerOpts, b.healthCoordinator != nil,
		secrets.WithHealthCoordinator(b.healthCoordinator))

	manager, err := secrets.New[any](provider, managerOpts...)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create secrets manager")
	}

	return manager, nil
}

// CreateVaultProvider creates a Vault secrets provider from configuration.
// It is a package-level function so callers that only need a single provider
// can use it without constructing a full [ManagerBuilder].
func CreateVaultProvider(client *vaultApi.Client, cfg *config.SecretsVault) (*vaultprovider.Storage[any], error) {
	if client == nil {
		return nil, fmt.Errorf("vault client is required")
	}

	var opts []vaultprovider.Option[any]
	if cfg != nil {
		opts = slices.AppendIf(opts, cfg.MountPath != "", vaultprovider.WithMountPath[any](cfg.MountPath))
		opts = slices.AppendIf(opts, cfg.SecretPath != "", vaultprovider.WithSecretPath[any](cfg.SecretPath))
		opts = slices.AppendIf(opts, cfg.CAS, vaultprovider.WithCas[any]())
	}

	return vaultprovider.New[any](client, opts...)
}

// CreateGCPProvider creates a GCP Secret Manager provider from configuration.
// It is a package-level function so callers that only need a single provider
// can use it without constructing a full [ManagerBuilder].
func CreateGCPProvider(ctx context.Context, cfg *config.SecretsGCP) (*gcpprovider.Storage[any], error) {
	var opts []gcpprovider.Option[any]
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, gcpprovider.WithLabels[any](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, gcpprovider.WithIgnoreInvalidKeys[any]())

	return gcpprovider.New[any](ctx, cfg.ProjectID, cfg.ServiceAccountPath, opts...)
}

// CreateLockboxProvider creates a Yandex Cloud Lockbox secrets provider from configuration.
// It is a package-level function so callers that only need a single provider
// can use it without constructing a full [ManagerBuilder].
func CreateLockboxProvider(ctx context.Context, cfg *config.SecretsLockbox) (*lockboxprovider.Storage[any], error) {
	// Read private key from file
	privKey, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to read Lockbox private key")
	}

	var opts []lockboxprovider.Option[any]
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, lockboxprovider.WithLabels[any](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, lockboxprovider.WithIgnoreInvalidKeys[any]())

	return lockboxprovider.New[any](ctx, cfg.FolderID, cfg.KeyID, cfg.ServiceKeyID, privKey, opts...)
}

// createProvider creates a secrets provider based on the configuration provider type.
func (b *ManagerBuilder) createProvider(ctx context.Context) (secrets.Provider[any], error) {
	switch b.cfg.Provider {
	case config.SecretsProviderVault:
		return CreateVaultProvider(b.vaultClient, b.cfg.Vault)
	case config.SecretsProviderGCP:
		return CreateGCPProvider(ctx, b.cfg.GCP)
	case config.SecretsProviderLockbox:
		return CreateLockboxProvider(ctx, b.cfg.Lockbox)
	case config.SecretsProviderMemory:
		return memoryprovider.New[any](nil)
	default:
		return nil, coreerrs.Wrapf(fmt.Errorf("unsupported provider: %s", b.cfg.Provider), "invalid configuration for %s", "secrets provider")
	}
}

// buildManagerOptions builds manager options from configuration.
func (b *ManagerBuilder) buildManagerOptions() ([]secrets.Option, error) {
	opts := []secrets.Option{
		secrets.WithLogger(b.Logger()),
		secrets.WithMaxRetries(b.cfg.Retry.MaxAttempts),
		secrets.WithExponentialConfig(retry.ExponentialConfig{
			BaseDelay: b.cfg.Retry.BaseDelay,
			MaxDelay:  b.cfg.Retry.MaxDelay,
			Factor:    b.cfg.Retry.Multiplier,
			Jitter:    b.cfg.Retry.Jitter,
		}),
	}

	return slices.AppendNonNilErr(opts, func() (secrets.Option, error) {
		cache, err := createCache(b.cfg.Cache)
		if err != nil || cache == nil {
			return nil, err
		}

		return secrets.WithCache[any](cache), nil
	})
}

// valueCache is the cache type a [secrets.Manager] of type any expects.
//
// The instantiation matters: [secrets.WithCache] accepts the cache as an `any`
// and the Manager silently falls back to its own default when the type
// assertion fails, so a mismatch here would look like a working cache that is
// never consulted.
type valueCache = secrets.Cache[string, *secrets.Value[any]]

// createCache builds the value cache described by cfg, or nil to leave the
// choice to the Manager.
//
// ShardCount selects the implementation: zero means one standard LRU, anything
// higher means a sharded one, which trades memory for reduced lock contention.
// MaxSize zero means the Manager's own default, so returning nil and letting it
// build its own cache is the faithful mapping — not a cache of size zero, which
// the LRU rejects outright.
func createCache(cfg config.SecretsCache) (valueCache, error) {
	if cfg.MaxSize <= 0 {
		return nil, nil //nolint:nilnil // no override; the Manager builds its default cache
	}

	if cfg.ShardCount <= 0 {
		cache, err := secrets.NewStandardCache[string, *secrets.Value[any]](cfg.MaxSize)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create secrets cache")
		}

		return cache, nil
	}

	cache, err := secrets.NewShardedCache[string, *secrets.Value[any]](
		cfg.MaxSize, secrets.WithShardCount(cfg.ShardCount))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create sharded secrets cache")
	}

	return cache, nil
}
