// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/factoryconv"

	bodylimitmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
	corsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
	geoaclmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
	idempotencymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
	ipaclmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
	limitermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
	loggermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
	metricsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/metrics"
	realipmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	recoverymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	requestidmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	securityheadersmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/securityheaders"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
	sharedreqid "github.com/altessa-s/go-atlas/transport/internal/requestid"
)

// middlewaresCfg returns the middlewares config or nil if not configured.
func (b *ServerBuilder) middlewaresCfg() *config.MiddlewaresConfig {
	if b.cfg == nil {
		return nil
	}
	return b.cfg.Middlewares
}

// WithMiddlewares creates all enabled middlewares from the builder's config
// (b.cfg.Middlewares) and adds them. If not called, no config-based middleware
// will be applied. Can be combined with WithMiddleware() and Without*Middleware().
//
// Authentication is intentionally NOT part of this set: unlike the gRPC server
// factory, no auth/scope middleware is installed here, because HTTP auth needs
// per-application configuration (token source, protected routes). Callers that
// require authentication must add it explicitly via WithMiddleware(auth.New(...))
// — relying on WithMiddlewares alone leaves routes unauthenticated.
//
// The optional exclude parameter accepts middleware references to skip.
// Each middleware package exports an ID variable that can be used here.
//
// Example:
//
//	// Register all middlewares except cors and limiter:
//	builder.WithMiddlewares(corsmw.ID, limitermw.ID)
func (b *ServerBuilder) WithMiddlewares(exclude ...middlewares.Middleware) *ServerBuilder {
	if b.middlewaresCfg() == nil {
		return b
	}

	skip := make(map[string]struct{}, len(exclude))
	for _, e := range exclude {
		skip[e.Name()] = struct{}{}
	}

	type entry struct {
		name string
		fn   func() *ServerBuilder
	}

	middlewares := []entry{
		{realipmw.Name(), b.WithRealIPMiddleware},
		{requestidmw.Name(), b.WithRequestIDMiddleware},
		{recoverymw.Name(), b.WithRecoveryMiddleware},
		{tracingmw.Name(), b.WithTracingMiddleware},
		{loggermw.Name(), b.WithLoggerMiddleware},
		{metricsmw.Name(), b.WithMetricsMiddleware},
		{corsmw.Name(), b.WithCorsMiddleware},
		{securityheadersmw.Name(), b.WithSecurityHeadersMiddleware},
		{bodylimitmw.Name(), b.WithBodyLimitMiddleware},
		{ipaclmw.Name(), b.WithIpAclMiddleware},
		{geoaclmw.Name(), b.WithGeoAclMiddleware},
		{limitermw.Name(), b.WithLimiterMiddleware},
		{idempotencymw.Name(), b.WithIdempotencyMiddleware},
	}

	for _, m := range middlewares {
		if _, excluded := skip[m.name]; !excluded {
			m.fn()
		}
	}

	return b
}

// WithBodyLimitMiddleware creates a body limit middleware from the builder's configuration.
func (b *ServerBuilder) WithBodyLimitMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.BodyLimit == nil || !cfg.BodyLimit.IsEnabled() {
		return b
	}
	var opts []bodylimitmw.Option
	opts = slices.AppendIf(opts, cfg.BodyLimit.RequireContentLength,
		bodylimitmw.WithRequireContentLength())
	b.configMW = append(b.configMW, bodylimitmw.New(cfg.BodyLimit.MaxSize, opts...))
	return b
}

// WithCorsMiddleware creates a CORS middleware from the builder's configuration.
func (b *ServerBuilder) WithCorsMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Cors == nil || !cfg.Cors.IsEnabled() {
		return b
	}

	c := cfg.Cors
	configOpts := []corsmw.Option{
		corsmw.WithLogger(b.Logger()),
		corsmw.WithAllowedOrigins(c.AllowedOrigins...),
		corsmw.WithMaxAge(c.MaxAge),
		corsmw.WithOptionsSuccessStatus(c.OptionsSuccessStatus),
	}

	configOpts = slices.AppendIf(configOpts, len(c.AllowedMethods) > 0, corsmw.WithAllowedMethods(c.AllowedMethods...))
	configOpts = slices.AppendIf(configOpts, len(c.AllowedHeaders) > 0, corsmw.WithAllowedHeaders(c.AllowedHeaders...))
	configOpts = slices.AppendIf(configOpts, len(c.ExposedHeaders) > 0, corsmw.WithExposedHeaders(c.ExposedHeaders...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePaths) > 0, corsmw.WithIgnorePaths(c.IgnorePaths...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		corsmw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = slices.AppendIf(configOpts, c.AllowCredentials, corsmw.WithAllowCredentials())
	configOpts = slices.AppendIf(configOpts, c.AllowPrivateNetwork, corsmw.WithAllowPrivateNetwork())
	configOpts = slices.AppendIf(configOpts, c.OptionsPassthrough, corsmw.WithOptionsPassthrough())

	b.configMW = append(b.configMW, corsmw.New(configOpts...))
	return b
}

// WithIdempotencyMiddleware creates an idempotency middleware from the builder's configuration.
func (b *ServerBuilder) WithIdempotencyMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Idempotency == nil || !cfg.Idempotency.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.idempotency, "idempotency keeper"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Idempotency
	configOpts := []idempotencymw.Option{
		idempotencymw.WithLogger(b.Logger()),
		idempotencymw.WithIdempotencyKeyHeader(c.IdempotencyKeyHeader),
		idempotencymw.WithIdempotencyKeyStatusHeader(c.IdempotencyKeyStatusHeader),
		idempotencymw.WithIdempotencyKeyEntityIDHeader(c.IdempotencyKeyEntityIdHeader),
		idempotencymw.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
		idempotencymw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		idempotencymw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = append(configOpts, idempotencymw.WithEnforceMandatory(c.EnforceMandatory))

	// ParseKeyLogMode falls back to hashed, so an unrecognized configured value
	// keeps raw keys out of the log rather than exposing them.
	configOpts = append(configOpts,
		idempotencymw.WithKeyLogMode(idempotencymw.ParseKeyLogMode(string(c.KeyLogMode))))

	b.configMW = append(b.configMW, idempotencymw.New(b.idempotency, configOpts...))
	return b
}

// WithIpAclMiddleware creates an IP access control middleware from the builder's configuration.
func (b *ServerBuilder) WithIpAclMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.IpAcl == nil || !cfg.IpAcl.IsEnabled() {
		return b
	}

	c := cfg.IpAcl
	registry, err := factoryconv.BuildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to build IP ACL registry"))
		return b
	}

	configOpts := []ipaclmw.Option{
		ipaclmw.WithLogger(b.Logger()),
		ipaclmw.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
		ipaclmw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		ipaclmw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.configMW = append(b.configMW, ipaclmw.New(registry, configOpts...))
	return b
}

// WithGeoAclMiddleware creates a geographic access control middleware from the builder's configuration.
func (b *ServerBuilder) WithGeoAclMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.GeoAcl == nil || !cfg.GeoAcl.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.geoResolver, "geo resolver"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.GeoAcl
	registry, err := factoryconv.BuildGeoAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to build GeoACL registry"))
		return b
	}

	configOpts := []geoaclmw.Option{
		geoaclmw.WithLogger(b.Logger()),
		geoaclmw.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
		geoaclmw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		geoaclmw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.configMW = append(b.configMW, geoaclmw.New(b.geoResolver, registry, configOpts...))
	return b
}

// WithLimiterMiddleware creates a rate limiter middleware from the builder's configuration.
func (b *ServerBuilder) WithLimiterMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Limiter == nil || !cfg.Limiter.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.limiter, "limiter"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Limiter
	configOpts := []limitermw.Option{
		limitermw.WithLogger(b.Logger()),
		limitermw.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
		limitermw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		limitermw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.configMW = append(b.configMW, limitermw.New(b.limiter, configOpts...))
	return b
}

// WithLoggerMiddleware creates a logging middleware from the builder's configuration.
func (b *ServerBuilder) WithLoggerMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Logger == nil || !cfg.Logger.IsEnabled() {
		return b
	}

	c := cfg.Logger
	configOpts := []loggermw.Option{
		loggermw.WithTimeFormat(c.TimeFormat),
		loggermw.WithIgnorePaths(c.IgnorePaths...),
		loggermw.WithIgnoreResponseCodes(c.IgnoreResponseCodes...),
		loggermw.WithLogResponseCodes(c.LogResponseCodes...),
		loggermw.WithIgnoreMethods(c.IgnoreHttpMethods...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		loggermw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = slices.AppendIf(configOpts, c.LogRequest, loggermw.WithLogRequest())
	configOpts = slices.AppendIf(configOpts, c.LogResponse, loggermw.WithLogResponse())

	b.configMW = append(b.configMW, loggermw.New(loggermw.Slog(b.Logger()), configOpts...))
	return b
}

// WithMetricsMiddleware creates a metrics middleware from the builder's configuration.
func (b *ServerBuilder) WithMetricsMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Metrics == nil || !cfg.Metrics.IsEnabled() {
		return b
	}

	c := cfg.Metrics
	configOpts := []metricsmw.Option{
		metricsmw.WithLogger(b.Logger()),
		metricsmw.WithCollector(b.Collector()),
		metricsmw.WithMetricsSubsystem(c.Subsystem),
		metricsmw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		metricsmw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = slices.AppendIf(configOpts, len(c.DurationBuckets) > 0, metricsmw.WithDurationBuckets(c.DurationBuckets))
	configOpts = slices.AppendIf(configOpts, len(c.SizeBuckets) > 0, metricsmw.WithSizeBuckets(c.SizeBuckets))

	configOpts = slices.AppendIf(configOpts, c.EnableSizeMetrics, metricsmw.WithEnableSizeMetrics())

	b.configMW = append(b.configMW, metricsmw.New(configOpts...))
	return b
}

// WithTracingMiddleware creates a tracing middleware from the builder's configuration.
func (b *ServerBuilder) WithTracingMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Tracing == nil || !cfg.Tracing.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.tracer, "tracer"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Tracing
	configOpts := []tracingmw.Option{
		tracingmw.WithLogger(b.Logger()),
		tracingmw.WithIgnorePaths(c.IgnorePaths...),
	}

	if len(c.IgnorePatterns) > 0 {
		patterns := factoryconv.CompilePatterns(c.IgnorePatterns)
		configOpts = slices.AppendIf(configOpts, len(patterns) > 0, tracingmw.WithIgnorePatterns(patterns...))
	}

	b.configMW = append(b.configMW, tracingmw.New(b.tracer, configOpts...))
	return b
}

// WithRealIPMiddleware creates a real IP extraction middleware from the builder's configuration.
func (b *ServerBuilder) WithRealIPMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.RealIp == nil || !cfg.RealIp.IsEnabled() {
		return b
	}

	c := cfg.RealIp
	extractorOpts := []clientip.Option{
		clientip.WithLogger(b.Logger()),
		clientip.WithHeaders(c.Headers...),
	}

	if len(c.TrustedProxies) > 0 {
		proxies, err := factoryconv.ParsePrefixes(c.TrustedProxies)
		if err != nil {
			b.errs = append(b.errs, b.WrapError(err, "failed to parse trusted proxies"))
			return b
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedProxies(proxies...))
	}

	extractor, err := clientip.NewExtractor(extractorOpts...)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to create real IP extractor"))
		return b
	}

	b.configMW = append(b.configMW, realipmw.New(extractor, b.Logger()))
	return b
}

// WithRecoveryMiddleware creates a panic recovery middleware from the builder's configuration.
func (b *ServerBuilder) WithRecoveryMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.Recovery == nil || !cfg.Recovery.IsEnabled() {
		return b
	}

	c := cfg.Recovery
	configOpts := []recoverymw.Option{
		recoverymw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, c.LogStack, recoverymw.WithLogStack())

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		recoverymw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.configMW = append(b.configMW, recoverymw.New(b.Logger(), configOpts...))
	return b
}

// WithRequestIDMiddleware creates a request ID middleware from the builder's configuration.
func (b *ServerBuilder) WithRequestIDMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.RequestId == nil || !cfg.RequestId.IsEnabled() {
		return b
	}

	var genOpts []sharedreqid.Option
	genOpts = slices.AppendIf(genOpts, cfg.RequestId.GenerateIfMissing, sharedreqid.WithGenerateIfMissing())

	gen := sharedreqid.NewGenerator(genOpts...)

	b.configMW = append(b.configMW, requestidmw.New(gen))
	return b
}

// WithSecurityHeadersMiddleware creates a security headers middleware from the builder's configuration.
func (b *ServerBuilder) WithSecurityHeadersMiddleware() *ServerBuilder {
	cfg := b.middlewaresCfg()
	if cfg == nil || cfg.SecurityHeaders == nil || !cfg.SecurityHeaders.IsEnabled() {
		return b
	}

	c := cfg.SecurityHeaders
	configOpts := []securityheadersmw.Option{
		securityheadersmw.WithFrameOptions(securityheadersmw.FrameOptions(c.FrameOptions)),
		securityheadersmw.WithReferrerPolicy(securityheadersmw.ReferrerPolicy(c.ReferrerPolicy)),
		securityheadersmw.WithContentSecurityPolicy(c.ContentSecurityPolicy),
		securityheadersmw.WithPermissionsPolicy(c.PermissionsPolicy),
		securityheadersmw.WithHstsMaxAge(c.HstsMaxAge),
		securityheadersmw.WithIgnorePaths(c.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		securityheadersmw.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = slices.AppendIf(configOpts, c.HstsEnabled, securityheadersmw.WithHstsEnabled())
	configOpts = slices.AppendIf(configOpts, c.HstsIncludeSubDomains, securityheadersmw.WithHstsIncludeSubDomains())
	configOpts = slices.AppendIf(configOpts, c.HstsPreload, securityheadersmw.WithHstsPreload())
	configOpts = slices.AppendIf(configOpts, c.ContentTypeNoSniff, securityheadersmw.WithContentTypeNoSniff())
	configOpts = slices.AppendIf(configOpts, c.XssProtectionDisabled, securityheadersmw.WithXSSProtectionDisabled())

	b.configMW = append(b.configMW, securityheadersmw.New(configOpts...))
	return b
}
