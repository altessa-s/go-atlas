// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
	tlsocsp "github.com/altessa-s/go-tlsutils/ocsp"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	tlsfile "github.com/altessa-s/go-tlsutils/providers/file"
	tlsletsencrypt "github.com/altessa-s/go-tlsutils/providers/letsencrypt"
)

// ErrInsecureSkipVerifyRejected is returned by [ProvidersBuilder.CreateClientConfig]
// when SkipVerify is true and the configured [tlsconfig.SkipVerifyMode] is
// [tlsconfig.SkipVerifyModeEnforce] (the default). Operators who genuinely
// need to skip verification must explicitly opt out via SkipVerifyMode=warn
// (logged) or SkipVerifyMode=disabled (silent, tests only).
var ErrInsecureSkipVerifyRejected = errors.New(
	"security/tlsutils: SkipVerify is true but SkipVerifyMode is enforce — refusing to disable certificate verification",
)

// ProviderEnv carries the dependencies a registered provider builder shares
// with the built-in providers.
type ProviderEnv struct {
	// Logger is the builder's logger.
	Logger *slog.Logger
	// OCSPStapler is the injected or config-built stapler; nil when OCSP is off.
	OCSPStapler tlsutils.OCSPStapler
	// CacheDir is the certificate cache root; a provider caches in its own
	// subdirectory. Empty disables caching.
	CacheDir string
}

// ProviderBuilder creates the TLS provider of one provider type from the
// provider configuration, whose section of that type is set.
type ProviderBuilder func(cfg *tlsconfig.Provider, env ProviderEnv) (tlsproviders.Provider, error)

// ProvidersBuilder assembles a [tlsproviders.Providers] registry step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ProvidersBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds the file and Let's Encrypt providers. The Vault and S3
// ones register their [ProviderBuilder] with [ProvidersBuilder.UseProvider]
// from factory/ext/tlsutils/<type>; a configured section
// without one fails at Build naming the package to add.
type ProvidersBuilder struct {
	corefactory.Base
	cfg  *tlsconfig.Provider
	errs []error

	// Dependencies
	ocspStapler tlsutils.OCSPStapler
	scheduler   schedtask.Registrar
	providers   map[tlsproviders.ProviderType]ProviderBuilder
	cacheDir    string
}

// New creates a [ProvidersBuilder] for the given TLS provider config.
// Config can be nil — Build returns an empty [tlsproviders.Providers] in that case.
func New(cfg *tlsconfig.Provider) *ProvidersBuilder {
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

	b.ensureOcspStaplerFromConfig()

	providers := &tlsproviders.Providers{}

	if b.cfg.File != nil {
		provider, err := b.createFileProvider()
		if err != nil {
			return nil, b.WrapError(err, "failed to create file provider")
		}
		providers.Register(provider)
	}

	if b.cfg.Vault != nil {
		provider, err := b.createRegisteredProvider(tlsproviders.ProviderTypeVault)
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
		provider, err := b.createRegisteredProvider(tlsproviders.ProviderTypeS3)
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
func (b *ProvidersBuilder) CreateClientConfig(cfg *tlsconfig.Client) (*tls.Config, error) {
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
		case tlsconfig.SkipVerifyModeEnforce:
			return nil, b.WrapError(ErrInsecureSkipVerifyRejected,
				"refusing to build TLS client config for server "+cfg.ServerName)
		case tlsconfig.SkipVerifyModeWarn:
			b.Logger().Warn("TLS certificate verification is disabled — connections are susceptible to man-in-the-middle attacks",
				slog.String("server_name", cfg.ServerName))
			tlsConfig.InsecureSkipVerify = true
		case tlsconfig.SkipVerifyModeDisabled:
			// Explicit operator/test opt-out. Still log it: an operator who
			// disables verification should see runtime evidence of it, matching
			// the Warn mode above. Use only in tests with localhost fixtures.
			b.Logger().Warn("TLS certificate verification disabled via TLSSkipVerifyModeDisabled (explicit opt-out) — "+
				"connections are susceptible to man-in-the-middle attacks",
				slog.String("server_name", cfg.ServerName))
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

// createRegisteredProvider creates the provider of typ with the builder
// registered for it.
func (b *ProvidersBuilder) createRegisteredProvider(typ tlsproviders.ProviderType) (tlsproviders.Provider, error) {
	build, ok := b.providers[typ]
	if !ok {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "tls provider %q is not registered: add factory/ext/tlsutils/%s "+
			"with UseProvider(%q, %sfactory.Builder(...))", typ, typ, typ, typ)
	}
	return build(b.cfg, ProviderEnv{Logger: b.Logger(), OCSPStapler: b.ocspStapler, CacheDir: b.cacheDir})
}

// createLetsEncryptProvider creates a Let's Encrypt TLS provider from configuration.
func (b *ProvidersBuilder) createLetsEncryptProvider() (*tlsletsencrypt.LetsEncrypt, error) {
	cfg := b.cfg.LetsEncrypt
	providerOpts := b.letsEncryptProviderOpts()
	opts := make([]tlsletsencrypt.Option, 0, 3+len(providerOpts))
	opts = append(opts,
		tlsletsencrypt.WithDomains(cfg.Domain...),
		tlsletsencrypt.WithEmail(cfg.Email),
		tlsletsencrypt.WithRenewBefore(cfg.RenewBefore),
	)

	opts = append(opts, providerOpts...)

	return tlsletsencrypt.New(opts...)
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

// letsEncryptProviderOpts returns common options for letsencrypt provider.
func (b *ProvidersBuilder) letsEncryptProviderOpts() []tlsletsencrypt.Option {
	var opts []tlsletsencrypt.Option
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsletsencrypt.Option {
		return []tlsletsencrypt.Option{tlsletsencrypt.WithLogger(b.Logger())}
	})
	opts = slices.AppendIfFunc(opts, b.cacheDir != "", func() []tlsletsencrypt.Option {
		return []tlsletsencrypt.Option{tlsletsencrypt.WithCacheDir(filepath.Join(b.cacheDir, "letsencrypt"))}
	})
	return opts
}

// ensureOcspStaplerFromConfig populates b.ocspStapler from b.cfg.OCSP
// when the operator enabled OCSP via YAML and no stapler was already
// injected programmatically via UseOcspStapler. The auto-built stapler
// inherits failureMode, compression, cache cap, and HTTP timeout. A
// non-empty refreshSchedule is registered on the injected scheduler
// here; the stapler library does not import one.
//
// A programmatically-injected stapler always wins — operators staging
// custom retry / HTTP-client plumbing don't lose it just because they
// also filled in the YAML block. An injected stapler is not registered
// for periodic refresh.
func (b *ProvidersBuilder) ensureOcspStaplerFromConfig() {
	if b.ocspStapler != nil {
		return
	}
	ocspCfg := b.cfg.OCSP
	if ocspCfg == nil || !ocspCfg.Enabled {
		return
	}

	opts := []tlsocsp.Option{
		tlsocsp.WithFailureMode(tlsocsp.FailureMode(ocspCfg.FailureMode)),
		tlsocsp.WithMaxCacheEntries(ocspCfg.MaxCacheEntries),
	}
	opts = slices.AppendIf(opts, ocspCfg.EnableCompression, tlsocsp.WithCompression())
	opts = slices.AppendIf(opts, ocspCfg.HTTPTimeout > 0,
		tlsocsp.WithHTTPClient(&http.Client{Timeout: ocspCfg.HTTPTimeout}))
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsocsp.Option {
		return []tlsocsp.Option{tlsocsp.WithLogger(b.Logger())}
	})

	stapler := tlsocsp.NewOCSPStapler(opts...)
	b.ocspStapler = stapler
	b.registerOcspRefresh(stapler, ocspCfg.RefreshSchedule)
}

// registerOcspRefresh registers the whole-cache OCSP refresh on the
// injected scheduler. A nil scheduler or a blank schedule leaves refresh
// lazy: GetOCSPStaple fetches on a cache miss. Register runs before the
// stapler is marked managed, so a failed registration keeps RunRefreshAll
// available. Build does not fail when registration fails.
func (b *ProvidersBuilder) registerOcspRefresh(stapler *tlsocsp.Stapler, schedule string) {
	schedule = strings.TrimSpace(schedule)
	if b.scheduler == nil || schedule == "" {
		return
	}
	err := b.scheduler.Register(context.Background(), schedtask.Config{
		ID:          "ocsp-stapler-refresh-all",
		Description: "Refresh all cached OCSP staples",
		Func:        stapler.RefreshAllFunc(),
		Schedule:    schedule,
		Priority:    schedtask.PriorityLow,
	})
	if err != nil {
		b.Logger().Warn("failed to register OCSP refresh task",
			slog.Any("error", coreerrs.WrapOperation(err, "register OCSP refresh task")))
		return
	}
	stapler.MarkSchedulerManaged()
	b.Logger().Debug("registered OCSP refresh task", slog.String("schedule", schedule))
}
