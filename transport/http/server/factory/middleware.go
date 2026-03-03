// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"

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
)

// WithMiddlewares creates all enabled middlewares from the builder's config
// (b.cfg.Middlewares) and adds them. If not called, no config-based middleware
// will be applied. Can be combined with WithMiddleware() and WithoutMiddleware().
func (b *ServerBuilder) WithMiddlewares() *ServerBuilder {
	if b.cfg == nil {
		return b
	}
	cfg := b.cfg.Middlewares
	if cfg == nil {
		return b
	}
	return b.
		WithRealIPMiddleware(cfg.RealIp).
		WithRequestIDMiddleware(cfg.RequestId).
		WithRecoveryMiddleware(cfg.Recovery).
		WithTracingMiddleware(cfg.Tracing).
		WithLoggerMiddleware(cfg.Logger).
		WithPrometheusMiddleware(cfg.Prometheus).
		WithCorsMiddleware(cfg.Cors).
		WithSecurityHeadersMiddleware(cfg.SecurityHeaders).
		WithBodyLimitMiddleware(cfg.BodyLimit).
		WithLimiterMiddleware(cfg.Limiter).
		WithIdempotencyMiddleware(cfg.Idempotency)
}

// WithBodyLimitMiddleware creates a body limit middleware from configuration.
func (b *ServerBuilder) WithBodyLimitMiddleware(cfg *config.HttpInterBodyLimitConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}
	b.configMW = append(b.configMW, bodylimitmw.New(cfg.MaxSize))
	return b
}

// WithCorsMiddleware creates a CORS middleware from configuration.
func (b *ServerBuilder) WithCorsMiddleware(cfg *config.HttpInterCorsConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	configOpts := []corsmw.Option{
		corsmw.WithLogger(b.Logger()),
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

	b.configMW = append(b.configMW, corsmw.New(configOpts...))
	return b
}

// WithIdempotencyMiddleware creates an idempotency middleware from configuration.
func (b *ServerBuilder) WithIdempotencyMiddleware(cfg *config.HttpInterIdempotencyConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.idempotency, "idempotency keeper"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	configOpts := []idempotencymw.Option{
		idempotencymw.WithLogger(b.Logger()),
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

	b.configMW = append(b.configMW, idempotencymw.New(b.idempotency, configOpts...))
	return b
}

// WithLimiterMiddleware creates a rate limiter middleware from configuration.
func (b *ServerBuilder) WithLimiterMiddleware(cfg *config.HttpInterLimiterConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.limiter, "limiter"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	configOpts := []limitermw.Option{
		limitermw.WithLogger(b.Logger()),
		limitermw.WithFallbackBehavior(convertFallbackBehavior(cfg.FallbackBehavior)),
		limitermw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, limitermw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	b.configMW = append(b.configMW, limitermw.New(b.limiter, configOpts...))
	return b
}

// WithLoggerMiddleware creates a logging middleware from configuration.
func (b *ServerBuilder) WithLoggerMiddleware(cfg *config.HttpInterLoggerConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
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

	b.configMW = append(b.configMW, loggermw.New(loggermw.Slog(b.Logger()), configOpts...))
	return b
}

// WithPrometheusMiddleware creates a Prometheus metrics middleware from configuration.
func (b *ServerBuilder) WithPrometheusMiddleware(cfg *config.HttpInterPrometheusConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	configOpts := []prometheusmw.Option{
		prometheusmw.WithLogger(b.Logger()),
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

	b.configMW = append(b.configMW, prometheusmw.New(configOpts...))
	return b
}

// WithTracingMiddleware creates a tracing middleware from configuration.
func (b *ServerBuilder) WithTracingMiddleware(cfg *config.HttpInterTracingConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.tracer, "tracer"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	configOpts := []tracingmw.Option{
		tracingmw.WithLogger(b.Logger()),
		tracingmw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		if len(patterns) > 0 {
			configOpts = append(configOpts, tracingmw.WithIgnorePatterns(patterns...))
		}
	}

	b.configMW = append(b.configMW, tracingmw.New(b.tracer, configOpts...))
	return b
}

// WithRealIPMiddleware creates a real IP extraction middleware from configuration.
func (b *ServerBuilder) WithRealIPMiddleware(cfg *config.HttpInterRealIpConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	extractorOpts := []clientip.Option{
		clientip.WithLogger(b.Logger()),
		clientip.WithHeaders(cfg.Headers...),
	}

	if len(cfg.TrustedProxies) > 0 {
		proxies, err := parsePrefixes(cfg.TrustedProxies)
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

// WithRecoveryMiddleware creates a panic recovery middleware from configuration.
func (b *ServerBuilder) WithRecoveryMiddleware(cfg *config.HttpInterRecoveryConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	configOpts := []recoverymw.Option{
		recoverymw.WithIgnorePaths(cfg.IgnorePaths...),
	}

	configOpts = slices.AppendIf(configOpts, cfg.LogStack, recoverymw.WithLogStack())

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, recoverymw.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	b.configMW = append(b.configMW, recoverymw.New(b.Logger(), configOpts...))
	return b
}

// WithRequestIDMiddleware creates a request ID middleware from configuration.
func (b *ServerBuilder) WithRequestIDMiddleware(cfg *config.HttpInterRequestIdConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
	}

	var genOpts []sharedreqid.Option
	genOpts = slices.AppendIf(genOpts, cfg.GenerateIfMissing, sharedreqid.WithGenerateIfMissing())

	gen := sharedreqid.NewGenerator(genOpts...)

	b.configMW = append(b.configMW, requestidmw.New(gen))
	return b
}

// WithSecurityHeadersMiddleware creates a security headers middleware from configuration.
func (b *ServerBuilder) WithSecurityHeadersMiddleware(cfg *config.HttpInterSecurityHeadersConfig) *ServerBuilder {
	if cfg == nil || !cfg.IsEnabled() {
		return b
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

	b.configMW = append(b.configMW, securityheadersmw.New(configOpts...))
	return b
}
