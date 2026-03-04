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

// ProvidersBuilder assembles a [tlsproviders.Providers] registry step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ProvidersBuilder.Build] time.
// The builder is not safe for concurrent use.
type ProvidersBuilder struct {
	corefactory.Base
	cfg  *config.TlsProvider
	errs []error

	// Dependencies
	ocspStapler tlsutils.OCSPStapler
	vaultClient *vaultApi.Client
	cacheDir    string
}

// New creates a [ProvidersBuilder] for the given TLS provider config.
// Config can be nil — Build returns an empty [tlsproviders.Providers] in that case.
func New(cfg *config.TlsProvider) *ProvidersBuilder {
	return &ProvidersBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// --- Build ---

// Build assembles the TLS providers registry. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ProvidersBuilder) Build() (*tlsproviders.Providers, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return &tlsproviders.Providers{}, nil
	}

	providers := &tlsproviders.Providers{}

	if b.cfg.File != nil {
		provider, err := b.createFileProvider()
		if err != nil {
			return nil, b.WrapError(err, "failed to create file provider")
		}
		providers.Register(provider)
	}

	if b.cfg.Vault != nil {
		provider, err := b.createVaultProvider()
		if err != nil {
			return nil, b.WrapError(err, "failed to create vault provider")
		}
		providers.Register(provider)
	}

	if b.cfg.LetsEncrypt != nil {
		provider, err := b.createLetsEncryptProvider()
		if err != nil {
			return nil, b.WrapError(err, "failed to create letsencrypt provider")
		}
		providers.Register(provider)
	}

	return providers, nil
}

// CreateClientConfig creates a TLS configuration for client connections from config.
// Returns nil, nil if the configuration is nil.
// This is a standalone utility that does not depend on the builder's TLS provider config.
func (b *ProvidersBuilder) CreateClientConfig(cfg *config.TlsClient) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil //nolint:nilnil
	}

	caPool, err := tlsutils.BuildCAPool(true, cfg.CACerts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to build CA pool")
	}

	var certs []tls.Certificate
	if cfg.Certificate != "" && cfg.PrivateKey != "" {
		cert, err := tlsutils.LoadFromFile(cfg.PrivateKey, cfg.Certificate, cfg.PrivateKeyPassword.Expose())
		if err != nil {
			return nil, b.WrapError(err, "failed to load client certificate")
		}
		certs = []tls.Certificate{*cert}
	}

	tlsConfig := tlsutils.DefaultClientTLSConfig(cfg.ServerName)
	tlsConfig.InsecureSkipVerify = cfg.SkipVerify
	if cfg.SkipVerify {
		b.Logger().Warn("TLS certificate verification is disabled — connections are susceptible to man-in-the-middle attacks",
			slog.String("server_name", cfg.ServerName))
	}
	tlsConfig.RootCAs = caPool
	if len(certs) > 0 {
		tlsConfig.Certificates = certs
	}

	return tlsConfig, nil
}

// createFileProvider creates a file-based TLS provider from configuration.
func (b *ProvidersBuilder) createFileProvider() (*tlsfile.File, error) {
	opts := b.fileProviderOpts()

	return tlsfile.NewWithCertAndKey(
		b.cfg.File.Certificate,
		b.cfg.File.PrivateKey,
		b.cfg.File.PrivateKeyPassword.Expose(),
		opts...,
	)
}

// createVaultProvider creates a Vault-based TLS provider from configuration.
func (b *ProvidersBuilder) createVaultProvider() (*tlsvault.Vault, error) {
	if err := b.RequireDependency(b.vaultClient, "vault client"); err != nil {
		return nil, err
	}

	cfg := b.cfg.Vault
	opts := []tlsvault.Option{
		tlsvault.WithVc(b.vaultClient),
		tlsvault.WithCommonName(cfg.CommonName),
		tlsvault.WithRole(cfg.Role),
		tlsvault.WithRenewBefore(cfg.RenewBefore),
	}

	opts = slices.AppendIf(opts, len(cfg.SubjectAlternativeNames) > 0, tlsvault.WithSubjectAlternativeNames(cfg.SubjectAlternativeNames...))
	opts = slices.AppendIf(opts, len(cfg.IPSubjectAlternativeNames) > 0, tlsvault.WithIpSubjectAlternativeNames(cfg.IPSubjectAlternativeNames...))

	opts = append(opts, b.vaultProviderOpts()...)

	return tlsvault.New(opts...)
}

// createLetsEncryptProvider creates a Let's Encrypt TLS provider from configuration.
func (b *ProvidersBuilder) createLetsEncryptProvider() (*tlsle.LetsEncrypt, error) {
	cfg := b.cfg.LetsEncrypt
	providerOpts := b.letsEncryptProviderOpts()
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
func (b *ProvidersBuilder) fileProviderOpts() []tlsfile.Option {
	var opts []tlsfile.Option
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsfile.Option {
		return []tlsfile.Option{tlsfile.WithLogger(b.Logger())}
	})
	opts = slices.AppendIfFunc(opts, b.ocspStapler != nil, func() []tlsfile.Option {
		return []tlsfile.Option{tlsfile.WithOcspStapler(b.ocspStapler)}
	})
	return opts
}

// vaultProviderOpts returns common options for vault provider.
func (b *ProvidersBuilder) vaultProviderOpts() []tlsvault.Option {
	var opts []tlsvault.Option
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithLogger(b.Logger())}
	})
	opts = slices.AppendIfFunc(opts, b.cacheDir != "", func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithCacheDir(filepath.Join(b.cacheDir, "vault"))}
	})
	opts = slices.AppendIfFunc(opts, b.ocspStapler != nil, func() []tlsvault.Option {
		return []tlsvault.Option{tlsvault.WithOcspStapler(b.ocspStapler)}
	})
	return opts
}

// letsEncryptProviderOpts returns common options for letsencrypt provider.
func (b *ProvidersBuilder) letsEncryptProviderOpts() []tlsle.Option {
	var opts []tlsle.Option
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsle.Option {
		return []tlsle.Option{tlsle.WithLogger(b.Logger())}
	})
	opts = slices.AppendIfFunc(opts, b.cacheDir != "", func() []tlsle.Option {
		return []tlsle.Option{tlsle.WithCacheDir(filepath.Join(b.cacheDir, "letsencrypt"))}
	})
	return opts
}
