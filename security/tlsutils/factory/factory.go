// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
	"path/filepath"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/security/tlsutils"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
	tlsle "github.com/altessa-s/go-atlas/security/tlsutils/providers/le"
	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
	vaultApi "github.com/hashicorp/vault/api"
)

// Factory creates TLS configurations and providers from configuration.
type Factory struct {
	corefactory.Base
	ocspStapler tlsutils.OCSPStapler
	vaultClient *vaultApi.Client
	cacheDir    string
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:        corefactory.NewBase(cfg.logger),
		ocspStapler: cfg.ocspStapler,
		vaultClient: cfg.vaultClient,
		cacheDir:    cfg.cacheDir,
	}
}

// CreateClientConfigFromConfig creates a TLS configuration for client connections from config.
// Returns nil, nil if the configuration is nil.
func (f *Factory) CreateClientConfigFromConfig(cfg *config.TlsClient) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil //nolint:nilnil
	}

	caPool, err := tlsutils.BuildCAPool(true, cfg.CACerts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to build CA pool")
	}

	var certs []tls.Certificate
	if cfg.Certificate != "" && cfg.PrivateKey != "" {
		cert, err := tlsutils.LoadFromFile(cfg.PrivateKey, cfg.Certificate, cfg.PrivateKeyPassword.Expose())
		if err != nil {
			return nil, f.WrapError(err, "failed to load client certificate")
		}
		certs = []tls.Certificate{*cert}
	}

	tlsConfig := tlsutils.DefaultClientTLSConfig(cfg.ServerName)
	tlsConfig.InsecureSkipVerify = cfg.SkipVerify
	if cfg.SkipVerify {
		f.Logger().Warn("TLS certificate verification is disabled — connections are susceptible to man-in-the-middle attacks",
			slog.String("server_name", cfg.ServerName))
	}
	tlsConfig.RootCAs = caPool
	if len(certs) > 0 {
		tlsConfig.Certificates = certs
	}

	return tlsConfig, nil
}

// CreateProvidersFromConfig creates a Providers registry with all providers from configuration.
func (f *Factory) CreateProvidersFromConfig(cfg *config.TlsProvider) (*tlsproviders.Providers, error) {
	if cfg == nil {
		return &tlsproviders.Providers{}, nil
	}

	providers := &tlsproviders.Providers{}

	if cfg.File != nil {
		provider, err := f.CreateFileProviderFromConfig(cfg.File)
		if err != nil {
			return nil, f.WrapError(err, "failed to create file provider")
		}
		providers.Register(provider)
	}

	if cfg.Vault != nil {
		provider, err := f.CreateVaultProviderFromConfig(cfg.Vault)
		if err != nil {
			return nil, f.WrapError(err, "failed to create vault provider")
		}
		providers.Register(provider)
	}

	if cfg.LetsEncrypt != nil {
		provider, err := f.CreateLetsEncryptProviderFromConfig(cfg.LetsEncrypt)
		if err != nil {
			return nil, f.WrapError(err, "failed to create letsencrypt provider")
		}
		providers.Register(provider)
	}

	return providers, nil
}

// CreateFileProviderFromConfig creates a file-based TLS provider from configuration.
// Panics if cfg is nil.
func (f *Factory) CreateFileProviderFromConfig(cfg *config.TlsProviderFile) (*tlsfile.File, error) {
	opts := f.fileProviderOpts()

	return tlsfile.NewWithCertAndKey(
		cfg.Certificate,
		cfg.PrivateKey,
		cfg.PrivateKeyPassword.Expose(),
		opts...,
	)
}

// CreateVaultProviderFromConfig creates a Vault-based TLS provider from configuration.
// Panics if cfg is nil.
func (f *Factory) CreateVaultProviderFromConfig(cfg *config.TlsProviderVault) (*tlsvault.Vault, error) {
	if err := f.RequireDependency(f.vaultClient, "vault client"); err != nil {
		return nil, err
	}

	opts := []tlsvault.Option{
		tlsvault.WithVc(f.vaultClient),
		tlsvault.WithCommonName(cfg.CommonName),
		tlsvault.WithRole(cfg.Role),
		tlsvault.WithRenewBefore(cfg.RenewBefore),
	}

	opts = slices.AppendIf(opts, len(cfg.SubjectAlternativeNames) > 0, tlsvault.WithSubjectAlternativeNames(cfg.SubjectAlternativeNames...))
	opts = slices.AppendIf(opts, len(cfg.IPSubjectAlternativeNames) > 0, tlsvault.WithIpSubjectAlternativeNames(cfg.IPSubjectAlternativeNames...))

	opts = append(opts, f.vaultProviderOpts()...)

	return tlsvault.New(opts...)
}

// CreateLetsEncryptProviderFromConfig creates a Let's Encrypt TLS provider from configuration.
// Panics if cfg is nil.
func (f *Factory) CreateLetsEncryptProviderFromConfig(cfg *config.TlsProviderLetsEncrypt) (*tlsle.LetsEncrypt, error) {
	providerOpts := f.letsEncryptProviderOpts()
	opts := make([]tlsle.Option, 0, 3+len(providerOpts))
	opts = append(opts,
		tlsle.WithDomains(cfg.Domain...),
		tlsle.WithEmail(cfg.Email),
		tlsle.WithRenewBefore(cfg.RenewBefore),
	)

	opts = append(opts, providerOpts...)

	return tlsle.New(opts...)
}

// fileProviderOpts returns common options for file provider.
func (f *Factory) fileProviderOpts() []tlsfile.Option {
	var opts []tlsfile.Option
	opts = slices.AppendIfFunc(opts, f.Logger() != nil, func() []tlsfile.Option {
		return []tlsfile.Option{tlsfile.WithLogger(f.Logger())}
	})
	opts = slices.AppendIfFunc(opts, f.ocspStapler != nil, func() []tlsfile.Option {
		return []tlsfile.Option{tlsfile.WithOcspStapler(f.ocspStapler)}
	})
	return opts
}

// vaultProviderOpts returns common options for vault provider.
func (f *Factory) vaultProviderOpts() []tlsvault.Option {
	var opts []tlsvault.Option
	opts = slices.AppendIfFunc(opts, f.Logger() != nil, func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithLogger(f.Logger())}
	})
	opts = slices.AppendIfFunc(opts, f.cacheDir != "", func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithCacheDir(filepath.Join(f.cacheDir, "vault"))}
	})
	opts = slices.AppendIfFunc(opts, f.ocspStapler != nil, func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithOcspStapler(f.ocspStapler)}
	})
	return opts
}

// letsEncryptProviderOpts returns common options for letsencrypt provider.
func (f *Factory) letsEncryptProviderOpts() []tlsle.Option {
	var opts []tlsle.Option
	opts = slices.AppendIfFunc(opts, f.Logger() != nil, func() []tlsle.Option {
		return []tlsle.Option{tlsle.WithLogger(f.Logger())}
	})
	opts = slices.AppendIfFunc(opts, f.cacheDir != "", func() []tlsle.Option {
		return []tlsle.Option{tlsle.WithCacheDir(filepath.Join(f.cacheDir, "letsencrypt"))}
	})
	return opts
}
