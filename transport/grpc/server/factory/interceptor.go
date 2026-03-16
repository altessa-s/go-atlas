// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/limiter"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/prometheus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"

	ipaclinter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
	tracinginter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
	sharedreqid "github.com/altessa-s/go-atlas/transport/internal/requestid"
)

// interceptorsCfg returns the interceptors config or nil if not configured.
func (b *ServerBuilder) interceptorsCfg() *config.InterceptorsConfig {
	if b.cfg == nil {
		return nil
	}
	return b.cfg.Interceptors
}

// WithInterceptors creates all enabled interceptors from the builder's config
// (b.cfg.Interceptors) and adds them. If not called, no config-based interceptors
// will be applied. Dependencies must be set via Use*() methods before calling this.
func (b *ServerBuilder) WithInterceptors() *ServerBuilder {
	if b.interceptorsCfg() == nil {
		return b
	}
	return b.
		WithErrStatusInterceptor().
		WithRealIPInterceptor().
		WithRequestIDInterceptor().
		WithRecoveryInterceptor().
		WithTracingInterceptor().
		WithLoggerInterceptor().
		WithPrometheusInterceptor().
		WithIpAclInterceptor().
		WithLimiterInterceptor().
		WithIdempotencyInterceptor().
		WithCacheInterceptor().
		WithAuthInterceptor().
		WithHealthInterceptor()
}

// WithLoggerInterceptor creates a logging interceptor from the builder's configuration.
func (b *ServerBuilder) WithLoggerInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Logger == nil || !cfg.Logger.IsEnabled() {
		return b
	}

	c := cfg.Logger
	configOpts := []logger.Option{
		logger.WithTimeFormat(c.TimeFormat),
	}

	configOpts = slices.AppendIf(configOpts, c.LogRequest, logger.WithLogRequest())
	configOpts = slices.AppendIf(configOpts, c.LogResponse, logger.WithLogResponse())
	configOpts = append(configOpts, logger.WithIgnoreMethods(c.IgnoreMethods...))
	configOpts = append(configOpts, logger.WithLogGrpcResponseCodes(c.LogGrpcResponseCodes...))
	configOpts = append(configOpts, logger.WithIgnoreGrpcResponseCodes(c.IgnoreGrpcResponseCodes...))
	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, logger.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	configOpts = slices.AppendIf(configOpts, c.EnableContextLogger, logger.WithContextLogger(b.Logger()))

	b.interceptors = append(b.interceptors, logger.ServerInterceptor(logger.Slog(b.Logger()), configOpts...))
	return b
}

// WithPrometheusInterceptor creates a Prometheus metrics interceptor from the builder's configuration.
func (b *ServerBuilder) WithPrometheusInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Prometheus == nil || !cfg.Prometheus.IsEnabled() {
		return b
	}

	c := cfg.Prometheus
	configOpts := []prometheus.Option{
		prometheus.WithLogger(b.Logger()),
		prometheus.WithNamespace(c.Namespace),
		prometheus.WithSubsystem(c.Subsystem),
		prometheus.WithStreamSamplingRate(c.StreamSamplingRate),
		prometheus.WithStreamSamplingStrategy(prometheus.StreamSamplingStrategy(c.StreamSamplingStrategy)),
	}

	configOpts = slices.AppendIf(configOpts, c.EnableSizeMetrics, prometheus.WithEnableSizeMetrics())
	configOpts = slices.AppendIf(configOpts, c.EnableStreamMetrics, prometheus.WithEnableStreamMetrics())
	if len(c.DurationBuckets) > 0 {
		configOpts = append(configOpts, prometheus.WithDurationBuckets(c.DurationBuckets))
	}
	if len(c.SizeBuckets) > 0 {
		configOpts = append(configOpts, prometheus.WithSizeBuckets(c.SizeBuckets))
	}
	configOpts = append(configOpts, prometheus.WithIgnoreMethods(c.IgnoreMethods...))
	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, prometheus.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, prometheus.ServerInterceptor(configOpts...))
	return b
}

// WithTracingInterceptor creates a tracing interceptor from the builder's configuration.
func (b *ServerBuilder) WithTracingInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Tracing == nil || !cfg.Tracing.IsEnabled() {
		return b
	}

	tracer := b.tracer
	if tracer == nil {
		tracer = tracing.Noop()
	}

	c := cfg.Tracing
	configOpts := []tracinginter.Option{
		tracinginter.WithLogger(b.Logger()),
		tracinginter.WithIgnoreMethods(c.IgnoreMethods...),
	}

	if len(c.IgnorePatterns) > 0 {
		patterns := compilePatterns(c.IgnorePatterns)
		if len(patterns) > 0 {
			configOpts = append(configOpts, tracinginter.WithIgnorePatterns(patterns...))
		}
	}

	b.interceptors = append(b.interceptors, tracinginter.ServerInterceptor(tracer, configOpts...))
	return b
}

// WithRealIPInterceptor creates a real IP extraction interceptor from the builder's configuration.
func (b *ServerBuilder) WithRealIPInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.RealIp == nil || !cfg.RealIp.IsEnabled() {
		return b
	}

	c := cfg.RealIp
	extractorOpts := []clientip.Option{
		clientip.WithLogger(b.Logger()),
		clientip.WithTrustedProxiesCount(c.TrustedProxiesCount),
	}

	if len(c.TrustedProxies) > 0 {
		proxies, err := parsePrefixes(c.TrustedProxies)
		if err != nil {
			b.errs = append(b.errs, b.WrapError(err, "failed to parse trusted proxies"))
			return b
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedProxies(proxies...))
	}

	if len(c.TrustedPeers) > 0 {
		peers, err := parsePrefixes(c.TrustedPeers)
		if err != nil {
			b.errs = append(b.errs, b.WrapError(err, "failed to parse trusted peers"))
			return b
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedPeers(peers...))
	}

	extractorOpts = append(extractorOpts, clientip.WithHeaders(c.Headers...))
	extractorOpts = append(extractorOpts, clientip.WithCacheSize(c.CacheSize))

	extractor, err := clientip.NewExtractor(extractorOpts...)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to create real IP extractor"))
		return b
	}

	configOpts := []realip.Option{realip.WithLogger(b.Logger())}
	b.interceptors = append(b.interceptors, realip.ServerInterceptor(extractor, configOpts...))
	return b
}

// WithRecoveryInterceptor creates a panic recovery interceptor from the builder's configuration.
func (b *ServerBuilder) WithRecoveryInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Recovery == nil || !cfg.Recovery.IsEnabled() {
		return b
	}

	c := cfg.Recovery
	configOpts := []recovery.Option{
		recovery.WithLogger(b.Logger()),
	}

	configOpts = append(configOpts, recovery.WithIgnoreMethods(c.IgnoreMethods...))
	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, recovery.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, recovery.ServerInterceptor(configOpts...))
	return b
}

// WithRequestIDInterceptor creates a request ID interceptor from the builder's configuration.
func (b *ServerBuilder) WithRequestIDInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.RequestId == nil || !cfg.RequestId.IsEnabled() {
		return b
	}

	var genOpts []sharedreqid.Option
	genOpts = slices.AppendIf(genOpts, cfg.RequestId.GenerateIfMissing, sharedreqid.WithGenerateIfMissing())

	gen := sharedreqid.NewGenerator(genOpts...)

	b.interceptors = append(b.interceptors, requestid.ServerInterceptor(gen))
	return b
}

// WithIpAclInterceptor creates an IP access control interceptor from the builder's configuration.
func (b *ServerBuilder) WithIpAclInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.IpAcl == nil || !cfg.IpAcl.IsEnabled() {
		return b
	}

	c := cfg.IpAcl
	registry, err := buildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to build IP ACL registry"))
		return b
	}

	configOpts := []ipaclinter.Option{
		ipaclinter.WithLogger(b.Logger()),
		ipaclinter.WithFallbackBehavior(convertFallbackBehavior(c.FallbackBehavior)),
	}

	configOpts = append(configOpts, ipaclinter.WithIgnoreMethods(c.IgnoreMethods...))
	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, ipaclinter.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, ipaclinter.ServerInterceptor(registry, configOpts...))
	return b
}

// WithLimiterInterceptor creates a rate limiter interceptor from the builder's configuration.
func (b *ServerBuilder) WithLimiterInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Limiter == nil || !cfg.Limiter.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.limiter, "limiter"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Limiter
	configOpts := []limiter.Option{
		limiter.WithLogger(b.Logger()),
		limiter.WithFallbackBehavior(convertFallbackBehavior(c.FallbackBehavior)),
	}

	configOpts = append(configOpts, limiter.WithIgnoreMethods(c.IgnoreMethods...))
	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, limiter.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, limiter.ServerInterceptor(b.limiter, configOpts...))
	return b
}

// WithIdempotencyInterceptor creates an idempotency interceptor from the builder's configuration.
func (b *ServerBuilder) WithIdempotencyInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Idempotency == nil || !cfg.Idempotency.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.idempotency, "idempotency keeper"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Idempotency
	configOpts := []idempotency.Option{
		idempotency.WithIdempotencyKeyHeader(c.IdempotencyKeyHeader),
		idempotency.WithIdempotencyKeyStatusMetadata(c.IdempotencyKeyStatusMetadata),
		idempotency.WithIdempotencyKeyEntityIdMetadata(c.IdempotencyKeyEntityIdMetadata),
		idempotency.WithFallbackBehavior(convertFallbackBehavior(c.FallbackBehavior)),
		idempotency.WithIgnoreMethods(c.IgnoreMethods...),
	}

	configOpts = slices.AppendIf(configOpts, c.EnforceMandatory, idempotency.WithEnforceMandatory())

	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, idempotency.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, idempotency.ServerInterceptor(b.idempotency, configOpts...))
	return b
}

// WithCacheInterceptor creates a cache interceptor from the builder's configuration.
func (b *ServerBuilder) WithCacheInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Cache == nil || !cfg.Cache.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.cacher, "cacher"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Cache
	opts := []cache.Option{
		cache.WithLogger(b.Logger()),
		cache.WithCacheHeaders(c.EnableCacheHeaders),
		cache.WithKeysPrefix(c.KeysPrefix),
		cache.WithCompressionPreset(convertCacheCompressionPreset(c.CompressionPreset)),
		cache.WithIgnoreMethods(c.IgnoreMethods...),
	}

	if len(c.KeyMetadata) > 0 {
		opts = append(opts, cache.WithMetadataKeys(c.KeyMetadata...))
	}

	if b.cacheMetadataProcessor != nil {
		opts = append(opts, cache.WithMetadataProcessor(b.cacheMetadataProcessor))
	}

	if len(c.IgnorePatterns) > 0 {
		opts = append(opts, cache.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	// Decision with separate TTLs
	opts = append(opts, cache.WithCacheDecision(
		cache.DefaultSuccessAndErrorDecision(c.SuccessTTL, c.ErrorTTL),
	))

	b.interceptors = append(b.interceptors, cache.ServerInterceptor(b.cacher, opts...))
	return b
}

// WithAuthInterceptor creates an authentication interceptor from the builder's configuration.
func (b *ServerBuilder) WithAuthInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Auth == nil || !cfg.Auth.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.authFn, "auth function"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Auth
	configOpts := []auth.Option{
		auth.WithLogger(b.Logger()),
		auth.WithAuthFn(b.authFn),
		auth.WithIgnoreMethods(c.IgnoreMethods...),
	}

	if b.clientAuth != nil {
		configOpts = append(configOpts, auth.WithClientAuth(b.clientAuth))
	}

	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, auth.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, auth.ServerInterceptor(configOpts...))
	return b
}

// WithHealthInterceptor creates a health check interceptor from the builder's configuration.
func (b *ServerBuilder) WithHealthInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Health == nil || !cfg.Health.IsEnabled() {
		return b
	}

	if err := b.RequireDependency(b.healthChecker, "health checker"); err != nil {
		b.errs = append(b.errs, err)
		return b
	}

	c := cfg.Health
	configOpts := []health.Option{
		health.WithLogger(b.Logger()),
		health.WithIgnoreMethods(c.IgnoreMethods...),
	}

	if len(c.IgnorePatterns) > 0 {
		configOpts = append(configOpts, health.WithIgnorePatterns(compilePatterns(c.IgnorePatterns)...))
	}

	b.interceptors = append(b.interceptors, health.ServerInterceptor(b.healthChecker, configOpts...))
	return b
}

// WithErrStatusInterceptor creates an error status interceptor from the builder's configuration.
func (b *ServerBuilder) WithErrStatusInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || !cfg.ErrStatus.IsEnabled() {
		return b
	}

	c := cfg.ErrStatus

	var finalizer errstatus.Finalizer
	if c.Domain != nil && *c.Domain != "" {
		finalizer = errstatus.DefaultFinalizerWithDomain(*c.Domain)
	} else {
		finalizer = errstatus.DefaultFinalizer
	}

	opts := []errstatus.Option{
		errstatus.WithFinalizer(finalizer),
	}

	if c.CacheSize != nil {
		opts = append(opts, errstatus.WithCacheSize(*c.CacheSize))
	}
	if c.CacheDisabled {
		opts = append(opts, errstatus.WithCacheDisabled())
	}
	if c.CacheOnlySentinel {
		opts = append(opts, errstatus.WithCacheOnlySentinel())
	}

	b.interceptors = append(b.interceptors, errstatus.ServerInterceptor(opts...))
	return b
}
