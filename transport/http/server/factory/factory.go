// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"
	"regexp"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	bodylimitmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
	corsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
	idempotencymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
	limitermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
	loggermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
	prometheusmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/prometheus"
	realipmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	recoverymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	requestidmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	securityheadersmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/securityheaders"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
	sharedreqid "github.com/altessa-s/go-atlas/transport/internal/requestid"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// Factory creates HTTP servers and middleware from configuration objects.
// Create instances with [New]. The factory is not safe for concurrent use;
// callers should create one factory per goroutine or synchronize externally.
type Factory struct {
	corefactory.Base
	tlsProviders *tlsproviders.Providers
	tracer       tracing.Tracer
	limiter      sharedlimiter.Limiter
	idempotency  idempotencydata.Idempotency
}

// New creates a new [Factory] with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:         corefactory.NewBase(cfg.logger),
		tlsProviders: cfg.tlsProviders,
		tracer:       cfg.tracer,
		limiter:      cfg.limiter,
		idempotency:  cfg.idempotency,
	}
}

// CreateServerFromConfig creates an HTTP [server.Server] from configuration.
// Returns an error if cfg is nil or TLS setup fails.
func (f *Factory) CreateServerFromConfig(
	cfg *config.Http,
	router router.Router,
) (*server.Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts := make([]server.Option, 0, 5)
	opts = append(opts,
		server.WithRouter(router),
		server.WithReadTimeout(cfg.ReadTimeout),
		server.WithWriteTimeout(cfg.WriteTimeout),
		server.WithIdleTimeout(cfg.IdleTimeout),
	)

	baseOpts := []baseserver.Option{
		baseserver.WithAddress(cfg.ListenAddress),
		baseserver.WithLogger(f.Logger()),
	}

	if cfg.TLS != nil {
		tlsConfig, err := f.buildServerTlsConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		baseOpts = append(baseOpts, baseserver.WithTlsConfig(tlsConfig))
	}

	opts = append(opts, server.WithBaseOptions(baseOpts...))

	return server.New(opts...)
}

// CreateBodyLimitMiddlewareFromConfig creates a body limit middleware from configuration.
func (f *Factory) CreateBodyLimitMiddlewareFromConfig(
	cfg *config.HttpInterBodyLimitConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}
	return bodylimitmw.New(cfg.MaxSize)
}

// CreateCorsMiddlewareFromConfig creates a CORS middleware from configuration.
func (f *Factory) CreateCorsMiddlewareFromConfig(
	cfg *config.HttpInterCorsConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	configOpts := []corsmw.Option{
		corsmw.WithLogger(f.Logger()),
		corsmw.WithAllowedOrigins(cfg.AllowedOrigins...),
		corsmw.WithMaxAge(cfg.MaxAge),
		corsmw.WithOptionsSuccessStatus(cfg.OptionsSuccessStatus),
	}

	if len(cfg.AllowedMethods) > 0 {
		configOpts = append(configOpts, corsmw.WithAllowedMethods(cfg.AllowedMethods...))
	}
	if len(cfg.AllowedHeaders) > 0 {
		configOpts = append(configOpts, corsmw.WithAllowedHeaders(cfg.AllowedHeaders...))
	}
	if len(cfg.ExposedHeaders) > 0 {
		configOpts = append(configOpts, corsmw.WithExposedHeaders(cfg.ExposedHeaders...))
	}
	if len(cfg.IgnorePaths) > 0 {
		configOpts = append(configOpts, corsmw.WithIgnorePaths(cfg.IgnorePaths...))
	}
	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, corsmw.WithIgnorePatterns(patterns...))
	}

	configOpts = slices.AppendIf(configOpts, cfg.AllowCredentials, corsmw.WithAllowCredentials())
	configOpts = slices.AppendIf(configOpts, cfg.AllowPrivateNetwork, corsmw.WithAllowPrivateNetwork())
	configOpts = slices.AppendIf(configOpts, cfg.OptionsPassthrough, corsmw.WithOptionsPassthrough())

	return corsmw.New(configOpts...)
}

// CreateIdempotencyMiddlewareFromInterConfig creates an idempotency middleware from interceptor configuration.
func (f *Factory) CreateIdempotencyMiddlewareFromInterConfig(
	cfg *config.HttpInterIdempotencyConfig,
	keeper idempotencydata.Idempotency,
) (middlewares.Middleware, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(keeper, "idempotency keeper"); err != nil {
		return nil, err
	}

	configOpts := []idempotencymw.Option{
		idempotencymw.WithLogger(f.Logger()),
		idempotencymw.WithIdempotencyKeyHeader(cfg.IdempotencyKeyHeader),
		idempotencymw.WithIdempotencyKeyStatusHeader(cfg.IdempotencyKeyStatusHeader),
		idempotencymw.WithIdempotencyKeyEntityIdHeader(cfg.IdempotencyKeyEntityIdHeader),
		idempotencymw.WithFallbackBehavior(convertFallbackBehavior(cfg.FallbackBehavior)),
		idempotencymw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, idempotencymw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	configOpts = slices.AppendIf(configOpts, cfg.EnforceMandatory, idempotencymw.WithEnforceMandatory())

	return idempotencymw.New(keeper, configOpts...), nil
}

// CreateLimiterMiddlewareFromInterConfig creates a rate limiter middleware from interceptor configuration.
func (f *Factory) CreateLimiterMiddlewareFromInterConfig(
	cfg *config.HttpInterLimiterConfig,
) (middlewares.Middleware, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(f.limiter, "limiter"); err != nil {
		return nil, err
	}

	configOpts := []limitermw.Option{
		limitermw.WithLogger(f.Logger()),
		limitermw.WithFallbackBehavior(convertFallbackBehavior(cfg.FallbackBehavior)),
		limitermw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, limitermw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	return limitermw.New(f.limiter, configOpts...), nil
}

// CreateLoggerMiddlewareFromConfig creates a logging middleware from configuration.
func (f *Factory) CreateLoggerMiddlewareFromConfig(
	cfg *config.HttpInterLoggerConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	configOpts := []loggermw.Option{
		loggermw.WithTimeFormat(cfg.TimeFormat),
		loggermw.WithIgnorePaths(cfg.IgnorePaths...),
		loggermw.WithIgnoreResponseCodes(cfg.IgnoreResponseCodes...),
		loggermw.WithLogResponseCodes(cfg.LogResponseCodes...),
		loggermw.WithIgnoreMethods(cfg.IgnoreHttpMethods...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, loggermw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	configOpts = slices.AppendIf(configOpts, cfg.LogRequest, loggermw.WithLogRequest())
	configOpts = slices.AppendIf(configOpts, cfg.LogResponse, loggermw.WithLogResponse())

	return loggermw.New(loggermw.Slog(f.Logger()), configOpts...)
}

// CreatePrometheusMiddlewareFromConfig creates a Prometheus metrics middleware from configuration.
func (f *Factory) CreatePrometheusMiddlewareFromConfig(
	cfg *config.HttpInterPrometheusConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	configOpts := []prometheusmw.Option{
		prometheusmw.WithLogger(f.Logger()),
		prometheusmw.WithNamespace(cfg.Namespace),
		prometheusmw.WithSubsystem(cfg.Subsystem),
		prometheusmw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, prometheusmw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	if len(cfg.DurationBuckets) > 0 {
		configOpts = append(configOpts, prometheusmw.WithDurationBuckets(cfg.DurationBuckets))
	}
	if len(cfg.SizeBuckets) > 0 {
		configOpts = append(configOpts, prometheusmw.WithSizeBuckets(cfg.SizeBuckets))
	}

	configOpts = slices.AppendIf(configOpts, cfg.EnableSizeMetrics, prometheusmw.WithEnableSizeMetrics())

	return prometheusmw.New(configOpts...)
}

// CreateTracingMiddlewareFromConfig creates a tracing middleware from configuration.
func (f *Factory) CreateTracingMiddlewareFromConfig(
	cfg *config.HttpInterTracingConfig,
) (middlewares.Middleware, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(f.tracer, "tracer"); err != nil {
		return nil, err
	}

	configOpts := []tracingmw.Option{
		tracingmw.WithLogger(f.Logger()),
		tracingmw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		if len(patterns) > 0 {
			configOpts = append(configOpts, tracingmw.WithIgnorePatterns(patterns...))
		}
	}

	return tracingmw.New(f.tracer, configOpts...), nil
}

// CreateRealIPMiddlewareFromConfig creates a real IP extraction middleware from configuration.
func (f *Factory) CreateRealIPMiddlewareFromConfig(
	cfg *config.HttpInterRealIpConfig,
) (middlewares.Middleware, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	extractorOpts := []clientip.Option{
		clientip.WithLogger(f.Logger()),
		clientip.WithHeaders(cfg.Headers...),
	}

	if len(cfg.TrustedProxies) > 0 {
		proxies, err := parsePrefixes(cfg.TrustedProxies)
		if err != nil {
			return nil, f.WrapError(err, "failed to parse trusted proxies")
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedProxies(proxies...))
	}

	extractor, err := clientip.NewExtractor(extractorOpts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to create real IP extractor")
	}

	return realipmw.New(extractor, f.Logger()), nil
}

// CreateRecoveryMiddlewareFromConfig creates a panic recovery middleware from configuration.
func (f *Factory) CreateRecoveryMiddlewareFromConfig(
	cfg *config.HttpInterRecoveryConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	configOpts := []recoverymw.Option{
		recoverymw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, cfg.LogStack, recoverymw.WithLogStack())

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, recoverymw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	return recoverymw.New(f.Logger(), configOpts...)
}

// CreateRequestIDMiddlewareFromConfig creates a request ID middleware from configuration.
func (f *Factory) CreateRequestIDMiddlewareFromConfig(
	cfg *config.HttpInterRequestIdConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	var genOpts []sharedreqid.Option
	genOpts = slices.AppendIf(genOpts, cfg.GenerateIfMissing, sharedreqid.WithGenerateIfMissing())

	gen := sharedreqid.NewGenerator(genOpts...)

	return requestidmw.New(gen)
}

// CreateSecurityHeadersMiddlewareFromConfig creates a security headers middleware from configuration.
func (f *Factory) CreateSecurityHeadersMiddlewareFromConfig(
	cfg *config.HttpInterSecurityHeadersConfig,
) middlewares.Middleware {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	configOpts := []securityheadersmw.Option{
		securityheadersmw.WithFrameOptions(securityheadersmw.FrameOptions(cfg.FrameOptions)),
		securityheadersmw.WithReferrerPolicy(securityheadersmw.ReferrerPolicy(cfg.ReferrerPolicy)),
		securityheadersmw.WithContentSecurityPolicy(cfg.ContentSecurityPolicy),
		securityheadersmw.WithPermissionsPolicy(cfg.PermissionsPolicy),
		securityheadersmw.WithHstsMaxAge(cfg.HstsMaxAge),
		securityheadersmw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, securityheadersmw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	configOpts = slices.AppendIf(configOpts, cfg.HstsEnabled, securityheadersmw.WithHstsEnabled())
	configOpts = slices.AppendIf(configOpts, cfg.HstsIncludeSubDomains, securityheadersmw.WithHstsIncludeSubDomains())
	configOpts = slices.AppendIf(configOpts, cfg.HstsPreload, securityheadersmw.WithHstsPreload())
	configOpts = slices.AppendIf(configOpts, cfg.ContentTypeNoSniff, securityheadersmw.WithContentTypeNoSniff())
	configOpts = slices.AppendIf(configOpts, cfg.XssProtectionDisabled, securityheadersmw.WithXssProtectionDisabled())

	return securityheadersmw.New(configOpts...)
}

// CreateMiddlewaresFromConfig creates all enabled middlewares from a MiddlewaresConfig.
// Middlewares are sorted by declared dependencies using topological sort,
// duplicates are removed. The ordering logic matches gRPC interceptors
// (see transport/grpc/interceptors.OrderServerInterceptors).
func (f *Factory) CreateMiddlewaresFromConfig(cfg *config.MiddlewaresConfig) ([]middlewares.Middleware, error) {
	if cfg == nil {
		return nil, nil //nolint:nilnil
	}

	var mws []middlewares.Middleware

	appendMW := func(fn func() (middlewares.Middleware, error)) error {
		m, err := fn()
		if err == nil {
			mws = slices.AppendNonNil(mws, m)
		}
		return err
	}

	// Collect all enabled middlewares. Insertion order defines the default
	// for middlewares without declared dependencies (stable topological sort).
	if err := appendMW(func() (middlewares.Middleware, error) { return f.CreateRealIPMiddlewareFromConfig(cfg.RealIp) }); err != nil {
		return nil, err
	}
	mws = slices.AppendNonNil(mws, f.CreateRequestIDMiddlewareFromConfig(cfg.RequestId))
	mws = slices.AppendNonNil(mws, f.CreateRecoveryMiddlewareFromConfig(cfg.Recovery))
	if err := appendMW(func() (middlewares.Middleware, error) { return f.CreateTracingMiddlewareFromConfig(cfg.Tracing) }); err != nil {
		return nil, err
	}
	mws = slices.AppendNonNil(mws, f.CreateLoggerMiddlewareFromConfig(cfg.Logger))
	mws = slices.AppendNonNil(mws, f.CreatePrometheusMiddlewareFromConfig(cfg.Prometheus))
	mws = slices.AppendNonNil(mws, f.CreateCorsMiddlewareFromConfig(cfg.Cors))
	mws = slices.AppendNonNil(mws, f.CreateSecurityHeadersMiddlewareFromConfig(cfg.SecurityHeaders))
	mws = slices.AppendNonNil(mws, f.CreateBodyLimitMiddlewareFromConfig(cfg.BodyLimit))
	if err := appendMW(func() (middlewares.Middleware, error) { return f.CreateLimiterMiddlewareFromInterConfig(cfg.Limiter) }); err != nil {
		return nil, err
	}
	if err := appendMW(func() (middlewares.Middleware, error) {
		return f.CreateIdempotencyMiddlewareFromInterConfig(cfg.Idempotency, f.idempotency)
	}); err != nil {
		return nil, err
	}

	// Order by declared dependencies and deduplicate.
	return middlewares.OrderMiddlewaresWithDedupe(mws)
}

// buildServerTlsConfig builds a TLS config from HTTP TLS configuration.
func (f *Factory) buildServerTlsConfig(cfg *config.HttpTls) (*tls.Config, error) {
	if f.tlsProviders == nil {
		return nil, f.Errorf("tls config provided but tls providers are not configured")
	}

	providerType := tlsproviders.ProviderType(cfg.ProviderType)
	provider, ok := f.tlsProviders.Get(providerType)
	if !ok {
		return nil, f.Errorf("tls provider %q not configured", cfg.ProviderType)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, f.WrapError(err, "failed to create TLS config from provider")
	}

	if cfg.MinTLSVersion == "1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	return tlsConfig, nil
}

// compilePatterns compiles string patterns to regexp.
func compilePatterns(patterns []string) []*regexp.Regexp {
	result := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			result = append(result, re)
		}
	}
	return result
}

// parsePrefixes parses string IP/CIDR prefixes to netip.Prefix.
var parsePrefixes = clientip.ParsePrefixes

// convertFallbackBehavior converts config FallbackBehavior to internal fallback.Behavior.
func convertFallbackBehavior(fb config.FallbackBehavior) fallback.Behavior {
	return fallback.ParseBehavior(string(fb))
}
