// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"

	"github.com/altessa-s/go-atlas/store/mongo/kms"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	kmsaws "github.com/altessa-s/go-atlas/store/mongo/kms/providers/aws"
	kmsazure "github.com/altessa-s/go-atlas/store/mongo/kms/providers/azure"
	kmsgcp "github.com/altessa-s/go-atlas/store/mongo/kms/providers/gcp"
	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
)

// ProviderFunc constructs a custom KMS provider from the selected config and injected TLS settings.
type ProviderFunc func(*mongoconfig.KMS, *tls.Config) (kms.MasterKeyProvider, error)

// ProviderBuilder assembles a [kms.MasterKeyProvider] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ProviderBuilder.Build] time.
// The builder is not safe for concurrent use.
type ProviderBuilder struct {
	corefactory.Base
	cfg  *mongoconfig.KMS
	errs []error

	// Dependencies
	tlsConfig *tls.Config
	providers map[mongoconfig.KMSProvider]ProviderFunc
}

// New creates a [ProviderBuilder] for the given KMS config.
// Config can be nil — the error surfaces at [ProviderBuilder.Build] time.
func New(cfg *mongoconfig.KMS) *ProviderBuilder {
	return &ProviderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a KMS provider from configuration.
// Errors from fluent methods are accumulated and reported here via [errors.Join].
func (b *ProviderBuilder) Build() (kms.MasterKeyProvider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	switch b.cfg.Provider {
	case mongoconfig.KMSProviderLocal:
		return b.createLocalProvider()
	case mongoconfig.KMSProviderAWS:
		return b.createAWSProvider()
	case mongoconfig.KMSProviderAzure:
		return b.createAzureProvider()
	case mongoconfig.KMSProviderGCP:
		return b.createGCPProvider()
	}
	if build, ok := b.providers[b.cfg.Provider]; ok && b.cfg.Provider != "" {
		return build(b.cfg, b.tlsConfig)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported KMS provider: %s", b.cfg.Provider)
}

func (b *ProviderBuilder) createLocalProvider() (kms.MasterKeyProvider, error) {
	if b.cfg.Local == nil {
		return nil, corefactory.ErrConfigRequired
	}

	var opts []kmslocal.Option
	opts = coreslices.AppendIf(opts, !b.cfg.Local.MasterKey.IsEmpty(), kmslocal.WithMasterKey(b.cfg.Local.MasterKey.Expose()))
	opts = coreslices.AppendIf(opts, b.cfg.Local.MasterKeyFile != "", kmslocal.WithMasterKeyFile(b.cfg.Local.MasterKeyFile))

	return kmslocal.New(opts...)
}

func (b *ProviderBuilder) createAWSProvider() (kms.MasterKeyProvider, error) {
	cfg := b.cfg.AWS
	if cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	var opts []kmsaws.Option
	opts = coreslices.AppendIf(opts, cfg.Region != nil, kmsaws.WithRegion(cfg.Region))
	opts = coreslices.AppendIf(opts, cfg.Endpoint != nil, kmsaws.WithEndpoint(cfg.Endpoint))
	opts = coreslices.AppendIf(opts, b.tlsConfig != nil, kmsaws.WithTLS(b.tlsConfig))

	return kmsaws.New(cfg.AccessKeyId, cfg.SecretAccessKey.Expose(), cfg.Key, opts...), nil
}

func (b *ProviderBuilder) createAzureProvider() (kms.MasterKeyProvider, error) {
	cfg := b.cfg.Azure
	if cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	var opts []kmsazure.Option
	opts = coreslices.AppendIf(opts, cfg.KeyVersion != nil, kmsazure.WithKeyVersion(cfg.KeyVersion))
	opts = coreslices.AppendIf(opts, cfg.KeyVaultEndpoint != nil, kmsazure.WithKeyVaultEndpoint(cfg.KeyVaultEndpoint))
	opts = coreslices.AppendIf(opts, b.tlsConfig != nil, kmsazure.WithTLS(b.tlsConfig))

	return kmsazure.New(cfg.ClientId, cfg.ClientSecret.Expose(), cfg.TenantId, cfg.KeyName, opts...), nil
}

func (b *ProviderBuilder) createGCPProvider() (kms.MasterKeyProvider, error) {
	cfg := b.cfg.GCP
	if cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	var opts []kmsgcp.Option
	opts = coreslices.AppendIf(opts, cfg.Endpoint != nil, kmsgcp.WithEndpoint(cfg.Endpoint))
	opts = coreslices.AppendIf(opts, cfg.AuthenticationEndpoint != nil, kmsgcp.WithAuthenticationEndpoint(cfg.AuthenticationEndpoint))
	opts = coreslices.AppendIf(opts, cfg.KeyVersion != nil, kmsgcp.WithKeyVersion(cfg.KeyVersion))
	opts = coreslices.AppendIf(opts, b.tlsConfig != nil, kmsgcp.WithTLS(b.tlsConfig))

	return kmsgcp.New(
		cfg.ProjectId, cfg.Email, cfg.PrivateKey.Expose(),
		cfg.Location, cfg.KeyRing, cfg.KeyName,
		opts...,
	), nil
}
