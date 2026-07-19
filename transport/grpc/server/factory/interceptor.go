// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/limiter"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/factoryconv"

	geoaclinter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/geoacl"
	ipaclinter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
	metricsint "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
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
//
// The optional exclude parameter accepts interceptor references to skip.
// Each interceptor package exports an ID variable that can be used here.
//
// Example:
//
//	// Register all interceptors except auth and cache:
//	builder.WithInterceptors(auth.ID, cache.ID)
func (b *ServerBuilder) WithInterceptors(exclude ...interceptors.Interceptor) *ServerBuilder {
	if b.interceptorsCfg() == nil {
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

	interceptors := []entry{
		{errstatus.Name(), b.WithErrStatusInterceptor},
		{realip.Name(), b.WithRealIPInterceptor},
		{requestid.Name(), b.WithRequestIDInterceptor},
		{recovery.Name(), b.WithRecoveryInterceptor},
		{tracinginter.Name(), b.WithTracingInterceptor},
		{logger.Name(), b.WithLoggerInterceptor},
		{metricsint.Name(), b.WithMetricsInterceptor},
		{ipaclinter.Name(), b.WithIpAclInterceptor},
		{geoaclinter.Name(), b.WithGeoAclInterceptor},
		{limiter.Name(), b.WithLimiterInterceptor},
		{idempotency.Name(), b.WithIdempotencyInterceptor},
		{cache.Name(), b.WithCacheInterceptor},
		{auth.Name(), b.WithAuthInterceptor},
		{fieldbehavior.Name(), b.WithFieldBehaviorInterceptor},
		{protovalidator.Name(), b.WithBufValidatorInterceptor},
		{health.Name(), b.WithHealthInterceptor},
	}

	for _, i := range interceptors {
		if _, excluded := skip[i.name]; !excluded {
			i.fn()
		}
	}

	return b
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
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		logger.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	configOpts = slices.AppendIf(configOpts, c.EnableContextLogger, logger.WithContextLogger(b.Logger()))

	b.interceptors = append(b.interceptors, logger.ServerInterceptor(logger.Slog(b.Logger()), configOpts...))
	return b
}

// WithMetricsInterceptor creates a metrics interceptor from the builder's configuration.
func (b *ServerBuilder) WithMetricsInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
	if cfg == nil || cfg.Metrics == nil || !cfg.Metrics.IsEnabled() {
		return b
	}

	c := cfg.Metrics
	configOpts := []metricsint.Option{
		metricsint.WithLogger(b.Logger()),
		metricsint.WithCollector(b.Collector()),
		metricsint.WithMetricsSubsystem(c.Subsystem),
		metricsint.WithStreamSamplingRate(c.StreamSamplingRate),
		metricsint.WithStreamSamplingStrategy(metricsint.StreamSamplingStrategy(c.StreamSamplingStrategy)),
	}

	configOpts = slices.AppendIf(configOpts, c.EnableSizeMetrics, metricsint.WithEnableSizeMetrics())
	configOpts = slices.AppendIf(configOpts, c.EnableStreamMetrics, metricsint.WithEnableStreamMetrics())
	configOpts = slices.AppendIf(configOpts, len(c.DurationBuckets) > 0, metricsint.WithDurationBuckets(c.DurationBuckets))
	configOpts = slices.AppendIf(configOpts, len(c.SizeBuckets) > 0, metricsint.WithSizeBuckets(c.SizeBuckets))
	configOpts = append(configOpts, metricsint.WithIgnoreMethods(c.IgnoreMethods...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		metricsint.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.interceptors = append(b.interceptors, metricsint.ServerInterceptor(configOpts...))
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
		patterns := factoryconv.CompilePatterns(c.IgnorePatterns)
		configOpts = slices.AppendIf(configOpts, len(patterns) > 0, tracinginter.WithIgnorePatterns(patterns...))
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
		proxies, err := factoryconv.ParsePrefixes(c.TrustedProxies)
		if err != nil {
			b.errs = append(b.errs, b.WrapError(err, "failed to parse trusted proxies"))
			return b
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedProxies(proxies...))
	}

	if len(c.TrustedPeers) > 0 {
		peers, err := factoryconv.ParsePrefixes(c.TrustedPeers)
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
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		recovery.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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
	registry, err := factoryconv.BuildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
	if err != nil {
		b.errs = append(b.errs, b.WrapError(err, "failed to build IP ACL registry"))
		return b
	}

	configOpts := []ipaclinter.Option{
		ipaclinter.WithLogger(b.Logger()),
		ipaclinter.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
	}

	configOpts = append(configOpts, ipaclinter.WithIgnoreMethods(c.IgnoreMethods...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		ipaclinter.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.interceptors = append(b.interceptors, ipaclinter.ServerInterceptor(registry, configOpts...))
	return b
}

// WithGeoAclInterceptor creates a geographic access control interceptor from the builder's configuration.
func (b *ServerBuilder) WithGeoAclInterceptor() *ServerBuilder {
	cfg := b.interceptorsCfg()
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

	configOpts := []geoaclinter.Option{
		geoaclinter.WithLogger(b.Logger()),
		geoaclinter.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
	}

	configOpts = append(configOpts, geoaclinter.WithIgnoreMethods(c.IgnoreMethods...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		geoaclinter.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

	b.interceptors = append(b.interceptors, geoaclinter.ServerInterceptor(b.geoResolver, registry, configOpts...))
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
		limiter.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
	}

	configOpts = append(configOpts, limiter.WithIgnoreMethods(c.IgnoreMethods...))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		limiter.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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
		idempotency.WithFallbackBehavior(factoryconv.ConvertFallbackBehavior(c.FallbackBehavior)),
		idempotency.WithIgnoreMethods(c.IgnoreMethods...),
	}

	configOpts = append(configOpts, idempotency.WithEnforceMandatory(c.EnforceMandatory))

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		idempotency.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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

	opts = slices.AppendIf(opts, len(c.KeyMetadata) > 0, cache.WithMetadataKeys(c.KeyMetadata...))
	opts = slices.AppendIf(opts, b.cacheMetadataProcessor != nil, cache.WithMetadataProcessor(b.cacheMetadataProcessor))
	opts = slices.AppendIf(opts, len(b.cacheMethodConfigs) > 0, cache.WithMethodConfig(b.cacheMethodConfigs...))
	opts = slices.AppendIf(opts, len(c.IgnorePatterns) > 0,
		cache.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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

	configOpts = slices.AppendIf(configOpts, b.clientAuth != nil, auth.WithClientAuth(b.clientAuth))
	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		auth.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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

	configOpts = slices.AppendIf(configOpts, len(c.IgnorePatterns) > 0,
		health.WithIgnorePatterns(factoryconv.CompilePatterns(c.IgnorePatterns)...))

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

	opts = slices.AppendIfFunc(opts, c.CacheSize != nil, func() []errstatus.Option {
		return []errstatus.Option{errstatus.WithCacheSize(*c.CacheSize)}
	})
	opts = slices.AppendIf(opts, c.CacheDisabled, errstatus.WithCacheDisabled())
	opts = slices.AppendIf(opts, c.CacheOnlySentinel, errstatus.WithCacheOnlySentinel())

	b.interceptors = append(b.interceptors, errstatus.ServerInterceptor(opts...))
	return b
}

// WithFieldBehaviorInterceptor adds the field_behavior sanitizer to the chain.
// It is always on (not config-gated): for protos without google.api.field_behavior
// annotations it is a no-op, and for annotated ones it keeps INPUT_ONLY fields out
// of responses and OUTPUT_ONLY/IDENTIFIER fields out of accepted requests. Ordering
// is handled by its Dependencies() (it runs after auth), so response sanitization is
// enforced by default rather than being opt-in.
func (b *ServerBuilder) WithFieldBehaviorInterceptor() *ServerBuilder {
	b.interceptors = append(b.interceptors, fieldbehavior.ServerInterceptor(
		fieldbehavior.WithLogger(b.Logger()),
	))
	return b
}

// WithBufValidatorInterceptor creates a protovalidator interceptor using the buf protovalidate library.
// This is a programmatic interceptor — it is not driven by config and must be called explicitly.
func (b *ServerBuilder) WithBufValidatorInterceptor() *ServerBuilder {
	b.interceptors = append(b.interceptors, protovalidator.ServerInterceptor(
		protovalidator.ValidatorFunc(bufhelpers.BuildValidator(
			bufhelpers.BuildValidationFilter(),
			bufhelpers.WithReasonCode(b.reasonCode),
		)),
		protovalidator.WithLogger(b.Logger()),
	))
	return b
}
