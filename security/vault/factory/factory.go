// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault"
	"github.com/altessa-s/go-atlas/security/vault/auth"
	"github.com/altessa-s/go-atlas/security/vault/auth/approle"
	"github.com/altessa-s/go-atlas/security/vault/auth/token"
	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	vaultApi "github.com/hashicorp/vault/api"
)

// Factory creates Vault clients from configuration.
type Factory struct {
	corefactory.Base
	tlsConfig         *tls.Config
	healthCoordinator *health.Coordinator
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:              corefactory.NewBase(cfg.logger),
		tlsConfig:         cfg.tlsConfig,
		healthCoordinator: cfg.healthCoordinator,
	}
}

// CreateVaultFromConfig creates a Vault client from configuration.
func (f *Factory) CreateVaultFromConfig(ctx context.Context, cfg *config.Vault) (*vault.Vault, error) {
	if err := f.RequireDependency(cfg.Auth, "auth configuration"); err != nil {
		return nil, err
	}

	authMethod, err := f.CreateAuthMethodFromConfig(cfg.Auth)
	if err != nil {
		return nil, f.WrapError(err, "failed to create auth method")
	}

	// Create vault API client with address from config
	vaultClient, err := f.createVaultClient(cfg)
	if err != nil {
		return nil, f.WrapError(err, "failed to create vault client")
	}

	opts := f.applyDefaults()
	opts = append(opts, vault.WithAuthMethod(authMethod), vault.WithVaultClient(vaultClient))

	return vault.New(ctx, opts...)
}

// CreateAuthMethodFromConfig creates an auth method from configuration.
// Panics if cfg is nil.
func (f *Factory) CreateAuthMethodFromConfig(cfg *config.VaultAuth) (auth.Method, error) {
	switch cfg.Method {
	case config.VaultAuthMethodToken:
		if err := f.RequireDependency(cfg.Token, "token config"); err != nil {
			return nil, err
		}
		return token.New(cfg.Token.Expose()), nil
	case config.VaultAuthMethodAppRole:
		if err := f.RequireDependency(cfg.Approle, "approle config"); err != nil {
			return nil, err
		}
		return approle.New(cfg.Approle.RoleId, cfg.Approle.SecretId.Expose(), approle.WithMountPath(cfg.Approle.MountPath)), nil
	case config.VaultAuthMethodUserPass:
		if err := f.RequireDependency(cfg.Userpass, "userpass config"); err != nil {
			return nil, err
		}
		return userpass.New(cfg.Userpass.Username, cfg.Userpass.Password.Expose(), userpass.WithMountPath(cfg.Userpass.MountPath)), nil
	default:
		return nil, f.Errorf("unknown vault auth method: %s", cfg.Method)
	}
}

// createVaultClient creates a Vault API client with the address and TLS config.
func (f *Factory) createVaultClient(cfg *config.Vault) (*vaultApi.Client, error) {
	vaultConfig := vaultApi.DefaultConfig()
	vaultConfig.Address = cfg.Address
	vaultConfig.MaxRetries = 3

	if f.tlsConfig != nil {
		if transport, ok := vaultConfig.HttpClient.Transport.(*http.Transport); ok {
			transport.TLSClientConfig = f.tlsConfig
		}
	}

	return vaultApi.NewClient(vaultConfig)
}

// applyDefaults returns factory default options.
func (f *Factory) applyDefaults() []vault.Option {
	return []vault.Option{
		vault.WithLogger(f.Logger()),
		vault.WithHealthCoordinator(f.healthCoordinator),
	}
}
