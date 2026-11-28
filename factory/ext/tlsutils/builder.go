// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/encoding/keys"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
	tlsocsp "github.com/altessa-s/go-tlsutils/ocsp"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	tlsfile "github.com/altessa-s/go-tlsutils/providers/file"
	tlsletsencrypt "github.com/altessa-s/go-tlsutils/providers/letsencrypt"
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

// Warmer is a provider that can obtain its certificates before the first handshake. [ProvidersBuilder.Build] calls
// Warm on every provider that implements it and fails, closing the providers, when one fails.
type Warmer interface {
	Warm(ctx context.Context) error
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
	ocspStapler       tlsutils.OCSPStapler
	scheduler         schedtask.Registrar
	providers         map[tlsproviders.ProviderType]ProviderBuilder
	cacheDir          string
	ocspTaskNamespace string
	ocspHTTPClient    tlsocsp.HTTPDoer
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
func (b *ProvidersBuilder) Build(ctx context.Context) (*tlsproviders.Providers, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return &tlsproviders.Providers{}, nil
	}

	// Each registry has its own OCSP cache unless the caller supplied one.
	assembly := *b
	stapler := assembly.ensureOcspStaplerFromConfig()
	providers := &tlsproviders.Providers{}
	err := assembly.buildProviders(providers)
	for provider := range providers.List() {
		if warmer, ok := provider.(Warmer); ok && err == nil {
			err = b.WrapError(warmer.Warm(ctx), "failed to warm the "+string(provider.Type())+" provider")
		}
	}
	if err != nil {
		providers.Close(context.WithoutCancel(ctx), func(_ tlsproviders.Provider, closeErr error) {
			err = errors.Join(err, closeErr)
		})
		return nil, err
	}
	if stapler != nil {
		assembly.registerOcspRefresh(ctx, stapler, assembly.cfg.OCSP.RefreshSchedule)
	}
	return providers, nil
}

func (b *ProvidersBuilder) buildProviders(providers *tlsproviders.Providers) error {
	if b.cfg.File != nil {
		provider, err := b.createFileProvider()
		if err != nil {
			return b.WrapError(err, "failed to create file provider")
		}
		providers.Register(provider)
	}

	if b.cfg.Vault != nil {
		provider, err := b.createRegisteredProvider(tlsproviders.ProviderTypeVault)
		if err != nil {
			return b.WrapError(err, "failed to create vault provider")
		}
		providers.Register(provider)
	}

	if b.cfg.LetsEncrypt != nil {
		provider, err := b.createLetsEncryptProvider()
		if err != nil {
			return b.WrapError(err, "failed to create letsencrypt provider")
		}
		providers.Register(provider)
	}

	if b.cfg.S3 != nil {
		provider, err := b.createRegisteredProvider(tlsproviders.ProviderTypeS3)
		if err != nil {
			return b.WrapError(err, "failed to create s3 provider")
		}
		providers.Register(provider)
	}

	return nil
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
// inherits failureMode, compression, cache cap, and HTTP timeout; an HTTP client
// injected with UseOCSPHTTPClient replaces the timeout-bound default. It returns
// the newly built stapler for registration once all providers are ready.
//
// A programmatically-injected stapler always wins — operators staging
// custom retry / HTTP-client plumbing don't lose it just because they
// also filled in the YAML block. An injected stapler is not registered
// for periodic refresh.
func (b *ProvidersBuilder) ensureOcspStaplerFromConfig() *tlsocsp.Stapler {
	if b.ocspStapler != nil {
		return nil
	}
	ocspCfg := b.cfg.OCSP
	if ocspCfg == nil || !ocspCfg.Enabled {
		return nil
	}

	opts := []tlsocsp.Option{
		tlsocsp.WithFailureMode(tlsocsp.FailureMode(ocspCfg.FailureMode)),
		tlsocsp.WithMaxCacheEntries(ocspCfg.MaxCacheEntries),
	}
	opts = slices.AppendIf(opts, ocspCfg.EnableCompression, tlsocsp.WithCompression())
	switch {
	case b.ocspHTTPClient != nil:
		opts = append(opts, tlsocsp.WithHTTPClient(b.ocspHTTPClient))
	case ocspCfg.HTTPTimeout > 0:
		opts = append(opts, tlsocsp.WithHTTPClient(&http.Client{Timeout: ocspCfg.HTTPTimeout}))
	}
	opts = slices.AppendIfFunc(opts, b.Logger() != nil, func() []tlsocsp.Option {
		return []tlsocsp.Option{tlsocsp.WithLogger(b.Logger())}
	})

	stapler := tlsocsp.NewOCSPStapler(opts...)
	b.ocspStapler = stapler
	return stapler
}

// registerOcspRefresh registers the whole-cache OCSP refresh on the
// injected scheduler. A nil scheduler or a blank schedule leaves refresh
// lazy: GetOCSPStaple fetches on a cache miss. Register runs before the
// stapler is marked managed, so a failed registration keeps RunRefreshAll
// available. Build does not fail when registration fails.
func (b *ProvidersBuilder) registerOcspRefresh(ctx context.Context, stapler *tlsocsp.Stapler, schedule string) {
	schedule = strings.TrimSpace(schedule)
	if b.scheduler == nil || schedule == "" {
		return
	}
	err := b.scheduler.Register(ctx, schedtask.Config{
		ID:          b.ocspRefreshTaskID(),
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

// ocspRefreshTaskID identifies the certificate sources. Rotating credentials
// or adjusting the refresh interval preserves the scheduler's task history.
func (b *ProvidersBuilder) ocspRefreshTaskID() string {
	parts := []string{b.ocspTaskNamespace, b.cacheDir}
	if f := b.cfg.File; f != nil {
		parts = append(parts, keys.Compose("file", f.Certificate, f.PrivateKey))
	}
	if v := b.cfg.Vault; v != nil {
		parts = append(parts, keys.Compose("vault", v.Role, v.CommonName,
			keys.Compose(v.SubjectAlternativeNames...), keys.Compose(v.IPSubjectAlternativeNames...)))
	}
	if l := b.cfg.LetsEncrypt; l != nil {
		parts = append(parts, keys.Compose("letsencrypt", l.Email, keys.Compose(l.Domain...)))
	}
	if s := b.cfg.S3; s != nil {
		parts = append(parts, keys.Compose("s3", s.Endpoint, s.Region, s.Bucket, s.CertificateKey, s.PrivateKeyKey))
	}
	const digestBytes = 12
	digest := keys.Digest([]byte(keys.Compose(parts...)))
	return "ocsp-stapler-refresh-all-" + hex.EncodeToString(digest[:digestBytes])
}
