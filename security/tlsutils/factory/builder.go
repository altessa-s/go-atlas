// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/security/tlsutils"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
	tlsle "github.com/altessa-s/go-atlas/security/tlsutils/providers/le"
	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	vaultApi "github.com/hashicorp/vault/api"
)

// ErrInsecureSkipVerifyRejected is returned by [ProvidersBuilder.CreateClientConfig]
// when SkipVerify is true and the configured [config.TLSSkipVerifyMode] is
// [config.TLSSkipVerifyModeEnforce] (the default). Operators who genuinely
// need to skip verification must explicitly opt out via SkipVerifyMode=warn
// (logged) or SkipVerifyMode=disabled (silent, tests only).
var ErrInsecureSkipVerifyRejected = errors.New("security/tlsutils: SkipVerify is true but SkipVerifyMode is enforce — refusing to disable certificate verification")

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
	s3Client    tlss3.S3API
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

	if b.cfg.S3 != nil {
		provider, err := b.createS3Provider()
		if err != nil {
			return nil, b.WrapError(err, "failed to create s3 provider")
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
	if cfg.SkipVerify {
		switch cfg.SkipVerifyMode {
		case config.TLSSkipVerifyModeEnforce:
			return nil, b.WrapError(ErrInsecureSkipVerifyRejected,
				"refusing to build TLS client config for server "+cfg.ServerName)
		case config.TLSSkipVerifyModeWarn:
			b.Logger().Warn("TLS certificate verification is disabled — connections are susceptible to man-in-the-middle attacks",
				slog.String("server_name", cfg.ServerName))
			tlsConfig.InsecureSkipVerify = true
		case config.TLSSkipVerifyModeDisabled:
			// Silent opt-out: explicit operator/test consent, no log,
			// no error. Use only in tests with localhost fixtures.
			tlsConfig.InsecureSkipVerify = true
		default:
			// Empty or unrecognized mode — fail safe with the Enforce
			// error. The YAML loader fills in the default:"enforce"
			// struct tag, so this path is reached only by programmatic
			// callers that built TlsClient by hand and forgot to set
			// SkipVerifyMode, or by a typo in YAML. Either way we want
			// the operator to commit to a value explicitly rather than
			// inheriting a silent fallback.
			return nil, b.WrapError(ErrInsecureSkipVerifyRejected,
				"SkipVerifyMode must be one of enforce/warn/disabled (got "+string(cfg.SkipVerifyMode)+")")
		}
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

// createS3Provider creates an S3-based TLS provider from configuration.
func (b *ProvidersBuilder) createS3Provider() (*tlss3.S3, error) {
	cfg := b.cfg.S3

	client, err := b.getOrCreateS3Client(cfg)
	if err != nil {
		return nil, err
	}

	opts := []tlss3.Option{
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(cfg.PollInterval),
	}

	if cfg.SSE != nil {
		opts = append(opts, tlss3.WithSseType(tlss3.SSEType(cfg.SSE.Type)))
		switch cfg.SSE.Type {
		case config.SSETypeConfigKMS:
			opts = append(opts, tlss3.WithSseKMSKeyID(cfg.SSE.KMSKeyID))
		case config.SSETypeConfigC:
			opts = append(opts, tlss3.WithSseCustomerKey(cfg.SSE.CustomerKey.Expose()))
			opts = append(opts, tlss3.WithSseCustomerKeyMD5(cfg.SSE.CustomerKeyMD5))
		case config.SSETypeConfigS3:
			// SSE-S3 is transparent on read — no additional options needed.
		}
	}

	opts = append(opts, b.s3ProviderOpts()...)

	return tlss3.New(cfg.Bucket, cfg.CertificateKey, cfg.PrivateKeyKey, cfg.PrivateKeyPassword.Expose(), opts...)
}

// getOrCreateS3Client returns the injected S3 client or creates a new one from config.
func (b *ProvidersBuilder) getOrCreateS3Client(cfg *config.TlsProviderS3) (tlss3.S3API, error) {
	if b.s3Client != nil {
		return b.s3Client, nil
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey.Expose(),
			cfg.SecretKey.Expose(),
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &cfg.Endpoint
		o.UsePathStyle = cfg.PathStyle
	})

	return client, nil
}

// s3ProviderOpts returns common options for S3 provider.
func (b *ProvidersBuilder) s3ProviderOpts() []tlss3.Option {
	var opts []tlss3.Option
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlss3.Option {
		return []tlss3.Option{tlss3.WithLogger(b.Logger())}
	})
	opts = slices.AppendIfFunc(opts, b.ocspStapler != nil, func() []tlss3.Option {
		return []tlss3.Option{tlss3.WithOcspStapler(b.ocspStapler)}
	})
	return opts
}
