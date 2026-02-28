// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"os"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/errors"
	"github.com/altessa-s/go-atlas/core/runtime/retry"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/secrets"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	gcpprovider "github.com/altessa-s/go-atlas/security/secrets/providers/gcp"
	lockboxprovider "github.com/altessa-s/go-atlas/security/secrets/providers/lockbox"
	memoryprovider "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
	vaultprovider "github.com/altessa-s/go-atlas/security/secrets/providers/vault"
	vaultApi "github.com/hashicorp/vault/api"
)

// Factory creates secrets managers and providers.
type Factory struct {
	corefactory.Base
	scheduler         corescheduler.TaskRegistrar
	healthCoordinator *health.Coordinator
	vaultClient       *vaultApi.Client
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:              corefactory.NewBase(cfg.logger),
		scheduler:         cfg.scheduler,
		healthCoordinator: cfg.healthCoordinator,
		vaultClient:       cfg.vaultClient,
	}
}

// CreateManagerFromConfig creates a secrets Manager from configuration.
// It automatically creates the appropriate provider based on cfg.Provider,
// configures retry settings, and passes scheduler configuration to the manager.
// The manager will register RunUpdateCycle task if scheduler is provided.
//
// Returns the manager and an error if provider creation fails.
//
// Example:
//
//	manager, err := f.CreateManagerFromConfig(ctx, &cfg.Secrets)
//	if err != nil {
//	    return err
//	}
//	defer manager.Shutdown()
func (f *Factory) CreateManagerFromConfig(ctx context.Context, cfg *config.Secrets) (*secrets.Manager[any], error) {
	// Create provider based on configuration
	provider, err := f.createProviderFromConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Build manager options from config
	managerOpts := f.buildManagerOptions(cfg)

	// Add scheduler and update schedule options if scheduler is available
	if f.scheduler != nil {
		managerOpts = append(managerOpts, secrets.WithScheduler(f.scheduler))
		managerOpts = append(managerOpts, secrets.WithUpdateSchedule(cfg.UpdateSchedule, cfg.RunOnStart))
	}

	if f.healthCoordinator != nil {
		managerOpts = append(managerOpts, secrets.WithHealthCoordinator(f.healthCoordinator))
	}

	manager, err := secrets.New[any](provider, managerOpts...)
	if err != nil {
		return nil, errors.WrapOperation(err, "create secrets manager")
	}

	return manager, nil
}

func (f *Factory) createProviderFromConfig(ctx context.Context, cfg *config.Secrets) (secrets.Provider[any], error) {
	switch cfg.Provider {
	case config.SecretsProviderVault:
		return f.createVaultProviderFromConfig(cfg.Vault)
	case config.SecretsProviderGCP:
		return f.createGCPProviderFromConfig(ctx, cfg.GCP)
	case config.SecretsProviderLockbox:
		return f.createLockboxProviderFromConfig(ctx, cfg.Lockbox)
	case config.SecretsProviderMemory:
		return memoryprovider.New[any](nil)
	default:
		return nil, errors.Wrapf(fmt.Errorf("unsupported provider: %s", cfg.Provider), "invalid configuration for %s", "secrets provider")
	}
}

// createVaultProviderFromConfig creates a Vault provider from configuration.
func (f *Factory) createVaultProviderFromConfig(cfg *config.SecretsVault) (*vaultprovider.Storage[any], error) {
	if err := f.RequireDependency(f.vaultClient, "vault client"); err != nil {
		return nil, err
	}

	var opts []vaultprovider.Option[any]
	if cfg != nil {
		opts = slices.AppendIf(opts, cfg.MountPath != "", vaultprovider.WithMountPath[any](cfg.MountPath))
		opts = slices.AppendIf(opts, cfg.SecretPath != "", vaultprovider.WithSecretPath[any](cfg.SecretPath))
		opts = slices.AppendIf(opts, cfg.CAS, vaultprovider.WithCas[any]())
	}

	return vaultprovider.New[any](f.vaultClient, opts...)
}

func (f *Factory) createGCPProviderFromConfig(ctx context.Context, cfg *config.SecretsGCP) (*gcpprovider.Storage[any], error) {
	var opts []gcpprovider.Option[any]
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, gcpprovider.WithLabels[any](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, gcpprovider.WithIgnoreInvalidKeys[any]())

	return gcpprovider.New[any](ctx, cfg.ProjectID, cfg.ServiceAccountPath, opts...)
}

// createLockboxProviderFromConfig creates a Lockbox provider from configuration.
func (f *Factory) createLockboxProviderFromConfig(ctx context.Context, cfg *config.SecretsLockbox) (*lockboxprovider.Storage[any], error) {
	// Read private key from file
	privKey, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read Lockbox private key")
	}

	var opts []lockboxprovider.Option[any]
	opts = slices.AppendIf(opts, len(cfg.Labels) > 0, lockboxprovider.WithLabels[any](cfg.Labels))
	opts = slices.AppendIf(opts, cfg.IgnoreInvalidKeys, lockboxprovider.WithIgnoreInvalidKeys[any]())

	return lockboxprovider.New[any](ctx, cfg.FolderID, cfg.KeyID, cfg.ServiceKeyID, privKey, opts...)
}

// buildManagerOptions builds manager options from configuration.
func (f *Factory) buildManagerOptions(cfg *config.Secrets) []secrets.Option {
	return []secrets.Option{
		secrets.WithLogger(f.Logger()),
		secrets.WithMaxRetries(cfg.Retry.MaxAttempts),
		secrets.WithExponentialConfig(retry.ExponentialConfig{
			BaseDelay: cfg.Retry.BaseDelay,
			MaxDelay:  cfg.Retry.MaxDelay,
			Factor:    cfg.Retry.Multiplier,
			Jitter:    cfg.Retry.Jitter,
		}),
	}
}

// CreateVaultProviderFromConfig creates a Vault secrets provider from configuration.
func (f *Factory) CreateVaultProviderFromConfig(cfg *config.SecretsVault) (*vaultprovider.Storage[any], error) {
	return f.createVaultProviderFromConfig(cfg)
}

// CreateGCPProviderFromConfig creates a GCP Secret Manager provider from configuration.
func (f *Factory) CreateGCPProviderFromConfig(ctx context.Context, cfg *config.SecretsGCP) (*gcpprovider.Storage[any], error) {
	return f.createGCPProviderFromConfig(ctx, cfg)
}

// CreateLockboxProviderFromConfig creates a Yandex Cloud Lockbox secrets provider from configuration.
func (f *Factory) CreateLockboxProviderFromConfig(ctx context.Context, cfg *config.SecretsLockbox) (*lockboxprovider.Storage[any], error) {
	return f.createLockboxProviderFromConfig(ctx, cfg)
}
