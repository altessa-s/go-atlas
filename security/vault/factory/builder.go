// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/approle"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"
	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	vaultApi "github.com/hashicorp/vault/api"
)

// VaultBuilder assembles a [vault.Vault] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [VaultBuilder.Build] time.
// The builder is not safe for concurrent use.
type VaultBuilder struct {
	corefactory.Base
	cfg  *config.Vault
	errs []error

	// Dependencies
	tlsConfig         *tls.Config
	healthCoordinator *health.Coordinator
	collector         metrics.Collector
}

// New creates a [VaultBuilder] for the given Vault config.
// Config can be nil — the error surfaces at [VaultBuilder.Build] time.
func New(cfg *config.Vault) *VaultBuilder {
	return &VaultBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// --- Build ---

// Build assembles the Vault client. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *VaultBuilder) Build(ctx context.Context) (*vault.Vault, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(b.cfg, "vault configuration"); err != nil {
		return nil, err
	}

	if err := b.RequireDependency(b.cfg.Auth, "auth configuration"); err != nil {
		return nil, err
	}

	authMethod, err := b.createAuthMethod()
	if err != nil {
		return nil, b.WrapError(err, "failed to create auth method")
	}

	vaultClient, err := b.createVaultClient()
	if err != nil {
		return nil, b.WrapError(err, "failed to create vault client")
	}

	opts := b.applyDefaults()
	opts = append(opts, vault.WithAuthMethod(authMethod), vault.WithVaultClient(vaultClient))

	return vault.New(ctx, opts...)
}

// createAuthMethod creates an auth method from configuration.
func (b *VaultBuilder) createAuthMethod() (auth.Method, error) {
	cfg := b.cfg.Auth
	switch cfg.Method {
	case config.VaultAuthMethodToken:
		if err := b.RequireDependency(cfg.Token, "token config"); err != nil {
			return nil, err
		}
		return token.New(cfg.Token.Expose()), nil
	case config.VaultAuthMethodAppRole:
		if err := b.RequireDependency(cfg.Approle, "approle config"); err != nil {
			return nil, err
		}
		return approle.New(cfg.Approle.RoleId, cfg.Approle.SecretId.Expose(), approle.WithMountPath(cfg.Approle.MountPath)), nil
	case config.VaultAuthMethodUserPass:
		if err := b.RequireDependency(cfg.Userpass, "userpass config"); err != nil {
			return nil, err
		}
		return userpass.New(cfg.Userpass.Username, cfg.Userpass.Password.Expose(), userpass.WithMountPath(cfg.Userpass.MountPath)), nil
	default:
		return nil, b.Errorf("unknown vault auth method: %s", cfg.Method)
	}
}

// createVaultClient creates a Vault API client with the address and TLS config.
func (b *VaultBuilder) createVaultClient() (*vaultApi.Client, error) {
	vaultConfig := vaultApi.DefaultConfig()
	vaultConfig.Address = b.cfg.Address
	vaultConfig.MaxRetries = 3

	if b.tlsConfig != nil {
		if transport, ok := vaultConfig.HttpClient.Transport.(*http.Transport); ok {
			transport.TLSClientConfig = b.tlsConfig
		}
	}

	return vaultApi.NewClient(vaultConfig)
}

// applyDefaults returns builder default options.
func (b *VaultBuilder) applyDefaults() []vault.Option {
	return []vault.Option{
		vault.WithLogger(b.Logger()),
		vault.WithHealthCoordinator(b.healthCoordinator),
		vault.WithCollector(b.collector),
	}
}
