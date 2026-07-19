// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/mongo/kms"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	kmsaws "github.com/altessa-s/go-atlas/data/mongo/kms/aws"
	kmsazure "github.com/altessa-s/go-atlas/data/mongo/kms/azure"
	kmsgcp "github.com/altessa-s/go-atlas/data/mongo/kms/gcp"
	kmslocal "github.com/altessa-s/go-atlas/data/mongo/kms/local"
)

// ProviderBuilder assembles a [kms.Provider] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ProviderBuilder.Build] time.
// The builder is not safe for concurrent use.
type ProviderBuilder struct {
	corefactory.Base
	cfg  *config.MongoKMS
	errs []error

	// Dependencies
	tlsConfig *tls.Config
}

// New creates a [ProviderBuilder] for the given KMS config.
// Config can be nil — the error surfaces at [ProviderBuilder.Build] time.
func New(cfg *config.MongoKMS) *ProviderBuilder {
	return &ProviderBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a KMS provider from configuration.
// Errors from fluent methods are accumulated and reported here via [errors.Join].
func (b *ProviderBuilder) Build() (kms.Provider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch b.cfg.Provider {
	case config.MongoKMSProviderLocal:
		return b.createLocalProvider()
	case config.MongoKMSProviderAmazon:
		return b.createAWSProvider()
	case config.MongoKMSProviderAzure:
		return b.createAzureProvider()
	case config.MongoKMSProviderGoogle:
		return b.createGCPProvider()
	default:
		return nil, b.Errorf("unsupported KMS provider: %s", b.cfg.Provider)
	}
}

func (b *ProviderBuilder) createLocalProvider() (kms.Provider, error) {
	if b.cfg.Local == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmslocal.Option
	opts = coreslices.AppendIf(opts, !b.cfg.Local.MasterKey.IsEmpty(), kmslocal.WithMasterKey(b.cfg.Local.MasterKey.Expose()))
	opts = coreslices.AppendIf(opts, b.cfg.Local.MasterKeyFile != "", kmslocal.WithMasterKeyFile(b.cfg.Local.MasterKeyFile))

	return kmslocal.New(opts...)
}

func (b *ProviderBuilder) createAWSProvider() (kms.Provider, error) {
	cfg := b.cfg.Amazon
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmsaws.Option
	opts = coreslices.AppendIf(opts, cfg.Region != nil, kmsaws.WithRegion(cfg.Region))
	opts = coreslices.AppendIf(opts, cfg.Endpoint != nil, kmsaws.WithEndpoint(cfg.Endpoint))
	opts = coreslices.AppendIf(opts, b.tlsConfig != nil, kmsaws.WithTLS(b.tlsConfig))

	return kmsaws.New(cfg.AccessKeyId, cfg.SecretAccessKey.Expose(), cfg.Key, opts...), nil
}

func (b *ProviderBuilder) createAzureProvider() (kms.Provider, error) {
	cfg := b.cfg.Azure
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmsazure.Option
	opts = coreslices.AppendIf(opts, cfg.KeyVersion != nil, kmsazure.WithKeyVersion(cfg.KeyVersion))
	opts = coreslices.AppendIf(opts, cfg.KeyVaultEndpoint != nil, kmsazure.WithKeyVaultEndpoint(cfg.KeyVaultEndpoint))
	opts = coreslices.AppendIf(opts, b.tlsConfig != nil, kmsazure.WithTLS(b.tlsConfig))

	return kmsazure.New(cfg.ClientId, cfg.ClientSecret.Expose(), cfg.TenantId, cfg.KeyName, opts...), nil
}

func (b *ProviderBuilder) createGCPProvider() (kms.Provider, error) {
	cfg := b.cfg.Google
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
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
