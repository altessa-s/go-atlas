// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/mongo/kms"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	kmsaws "github.com/altessa-s/go-atlas/data/mongo/kms/aws"
	kmsazure "github.com/altessa-s/go-atlas/data/mongo/kms/azure"
	kmsgcp "github.com/altessa-s/go-atlas/data/mongo/kms/gcp"
	kmslocal "github.com/altessa-s/go-atlas/data/mongo/kms/local"
	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
)

// Factory creates KMS providers from configuration.
type Factory struct {
	corefactory.Base
	tlsFactory *tlsfactory.Factory
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:       corefactory.NewBase(cfg.logger),
		tlsFactory: cfg.tlsFactory,
	}
}

// CreateProviderFromConfig creates a KMS provider from configuration.
func (f *Factory) CreateProviderFromConfig(cfg *config.MongoKMS) (kms.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	switch cfg.Provider {
	case config.MongoKMSProviderLocal:
		return f.createLocalProvider(cfg.Local)
	case config.MongoKMSProviderAmazon:
		return f.createAWSProvider(cfg.Amazon)
	case config.MongoKMSProviderAzure:
		return f.createAzureProvider(cfg.Azure)
	case config.MongoKMSProviderGoogle:
		return f.createGCPProvider(cfg.Google)
	default:
		return nil, f.Errorf("unsupported KMS provider: %s", cfg.Provider)
	}
}

func (f *Factory) createLocalProvider(cfg *config.MongoKMSLocal) (kms.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmslocal.Option

	if !cfg.MasterKey.IsEmpty() {
		opts = append(opts, kmslocal.WithMasterKey(cfg.MasterKey.Expose()))
	}

	if cfg.MasterKeyFile != "" {
		opts = append(opts, kmslocal.WithMasterKeyFile(cfg.MasterKeyFile))
	}

	return kmslocal.New(opts...)
}

func (f *Factory) createAWSProvider(cfg *config.MongoKMSAmazon) (kms.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmsaws.Option

	if cfg.Region != nil {
		opts = append(opts, kmsaws.WithRegion(cfg.Region))
	}

	if cfg.Endpoint != nil {
		opts = append(opts, kmsaws.WithEndpoint(cfg.Endpoint))
	}

	if cfg.TLS != nil {
		tlsCfg, err := f.createTLSConfig(cfg.TLS)
		if err != nil {
			return nil, f.WrapError(err, "failed to create TLS config for AWS KMS")
		}
		opts = append(opts, kmsaws.WithTLS(tlsCfg))
	}

	return kmsaws.New(cfg.AccessKeyId, cfg.SecretAccessKey.Expose(), cfg.Key, opts...), nil
}

func (f *Factory) createAzureProvider(cfg *config.MongoKMSAzure) (kms.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmsazure.Option

	if cfg.KeyVersion != nil {
		opts = append(opts, kmsazure.WithKeyVersion(cfg.KeyVersion))
	}

	if cfg.KeyVaultEndpoint != nil {
		opts = append(opts, kmsazure.WithKeyVaultEndpoint(cfg.KeyVaultEndpoint))
	}

	if cfg.TLS != nil {
		tlsCfg, err := f.createTLSConfig(cfg.TLS)
		if err != nil {
			return nil, f.WrapError(err, "failed to create TLS config for Azure KMS")
		}
		opts = append(opts, kmsazure.WithTLS(tlsCfg))
	}

	return kmsazure.New(cfg.ClientId, cfg.ClientSecret.Expose(), cfg.TenantId, cfg.KeyName, opts...), nil
}

func (f *Factory) createGCPProvider(cfg *config.MongoKMSGoogle) (kms.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []kmsgcp.Option

	if cfg.Endpoint != nil {
		opts = append(opts, kmsgcp.WithEndpoint(cfg.Endpoint))
	}

	if cfg.AuthenticationEndpoint != nil {
		opts = append(opts, kmsgcp.WithAuthenticationEndpoint(cfg.AuthenticationEndpoint))
	}

	if cfg.KeyVersion != nil {
		opts = append(opts, kmsgcp.WithKeyVersion(cfg.KeyVersion))
	}

	if cfg.TLS != nil {
		tlsCfg, err := f.createTLSConfig(cfg.TLS)
		if err != nil {
			return nil, f.WrapError(err, "failed to create TLS config for GCP KMS")
		}
		opts = append(opts, kmsgcp.WithTLS(tlsCfg))
	}

	return kmsgcp.New(
		cfg.ProjectId, cfg.Email, cfg.PrivateKey.Expose(),
		cfg.Location, cfg.KeyRing, cfg.KeyName,
		opts...,
	), nil
}

func (f *Factory) createTLSConfig(cfg *config.TlsClient) (*tls.Config, error) {
	if err := f.RequireDependency(f.tlsFactory, "tls factory"); err != nil {
		return nil, err
	}
	return f.tlsFactory.CreateClientConfigFromConfig(cfg)
}
