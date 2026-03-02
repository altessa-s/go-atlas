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
	"github.com/altessa-s/go-atlas/security/tlsutils"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/limiter"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/prometheus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tracinginter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	sharedreqid "github.com/altessa-s/go-atlas/transport/internal/requestid"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// Factory creates gRPC servers and interceptors from [config] structs.
// All components inherit the factory's logger. When TLS is required, the
// factory resolves certificates through configured [tlsproviders.Providers].
type Factory struct {
	corefactory.Base
	cacheMetadataProcessor cache.MetadataProcessor
	tlsProviders           *tlsproviders.Providers
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:                   corefactory.NewBase(cfg.logger),
		cacheMetadataProcessor: cfg.cacheMetadataProcessor,
		tlsProviders:           cfg.tlsProviders,
	}
}

// CreateServerFromConfig builds a [grpcserver.Server] from cfg, applying TLS,
// keepalive, message size limits, and reflection settings. Returns an error
// if cfg is nil or if TLS provider resolution fails.
func (f *Factory) CreateServerFromConfig(cfg *config.Grpc) (*grpcserver.Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	var opts []grpcserver.Option

	opts = slices.AppendIf(opts, cfg.Reflection, grpcserver.WithReflection())

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

	opts = append(opts, grpcserver.WithBaseOptions(baseOpts...))

	// Add gRPC server options from config
	grpcOpts := f.buildGrpcOptions(cfg)
	opts = slices.AppendIf(opts, len(grpcOpts) > 0, grpcserver.WithGrpcOptions(grpcOpts...))

	return grpcserver.New(opts...)
}

// buildServerTlsConfig builds a TLS config from gRPC TLS configuration.
func (f *Factory) buildServerTlsConfig(cfg *config.GrpcTls) (*tls.Config, error) {
	if err := f.RequireDependency(f.tlsProviders, "tls providers"); err != nil {
		return nil, err
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

	// mTLS client authentication
	if cfg.Client != nil && cfg.Client.AllowCertAuth {
		caPool, err := tlsutils.BuildCAPool(false, cfg.Client.CACerts...)
		if err != nil {
			return nil, f.WrapError(err, "failed to build client CA pool")
		}
		tlsConfig.ClientCAs = caPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// buildGrpcOptions builds gRPC server options from configuration.
func (f *Factory) buildGrpcOptions(cfg *config.Grpc) []grpc.ServerOption {
	var opts []grpc.ServerOption

	if cfg.ConnectionTimeout != nil {
		opts = append(opts, grpc.ConnectionTimeout(*cfg.ConnectionTimeout))
	}

	if cfg.MaxConcurrentStreams != nil {
		opts = append(opts, grpc.MaxConcurrentStreams(*cfg.MaxConcurrentStreams))
	}

	if cfg.MaxSendMsgSize != nil {
		opts = append(opts, grpc.MaxSendMsgSize(*cfg.MaxSendMsgSize))
	}

	if cfg.MaxRecvMsgSize != nil {
		opts = append(opts, grpc.MaxRecvMsgSize(*cfg.MaxRecvMsgSize))
	}

	if cfg.WriteBufferSize != nil {
		opts = append(opts, grpc.WriteBufferSize(*cfg.WriteBufferSize))
	}

	if cfg.ReadBufferSize != nil {
		opts = append(opts, grpc.ReadBufferSize(*cfg.ReadBufferSize))
	}

	if cfg.KeepAlive != nil {
		opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     cfg.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      cfg.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: cfg.KeepAlive.MaxConnectionAgeGrace,
			Time:                  cfg.KeepAlive.Time,
			Timeout:               cfg.KeepAlive.Timeout,
		}))

		if cfg.KeepAlive.EnforcementPolicy != nil {
			opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             cfg.KeepAlive.EnforcementPolicy.MinTime,
				PermitWithoutStream: cfg.KeepAlive.EnforcementPolicy.PermitWithoutStream,
			}))
		}
	}

	return opts
}

// CreateLoggerInterceptorFromConfig creates a logging interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateLoggerInterceptorFromConfig(
	cfg *config.GrpcInterLoggerConfig,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	configOpts := []logger.Option{
		logger.WithTimeFormat(cfg.TimeFormat),
	}

	configOpts = slices.AppendIf(configOpts, cfg.LogRequest, logger.WithLogRequest())
	configOpts = slices.AppendIf(configOpts, cfg.LogResponse, logger.WithLogResponse())
	configOpts = append(configOpts, logger.WithIgnoreMethods(cfg.IgnoreMethods...))
	configOpts = append(configOpts, logger.WithLogGrpcResponseCodes(cfg.LogGrpcResponseCodes...))
	configOpts = append(configOpts, logger.WithIgnoreGrpcResponseCodes(cfg.IgnoreGrpcResponseCodes...))
	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, logger.WithIgnorePatterns(patterns...))
	}

	configOpts = slices.AppendIf(configOpts, cfg.EnableContextLogger, logger.WithContextLogger(f.Logger()))

	return logger.ServerInterceptor(logger.Slog(f.Logger()), configOpts...), nil
}

// CreatePrometheusInterceptorFromConfig creates a Prometheus metrics interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreatePrometheusInterceptorFromConfig(
	cfg *config.GrpcInterPrometheusConfig,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	configOpts := []prometheus.Option{
		prometheus.WithLogger(f.Logger()),
		prometheus.WithNamespace(cfg.Namespace),
		prometheus.WithSubsystem(cfg.Subsystem),
		prometheus.WithStreamSamplingRate(cfg.StreamSamplingRate),
		prometheus.WithStreamSamplingStrategy(prometheus.StreamSamplingStrategy(cfg.StreamSamplingStrategy)),
	}

	configOpts = slices.AppendIf(configOpts, cfg.EnableSizeMetrics, prometheus.WithEnableSizeMetrics())
	configOpts = slices.AppendIf(configOpts, cfg.EnableStreamMetrics, prometheus.WithEnableStreamMetrics())
	if len(cfg.DurationBuckets) > 0 {
		configOpts = append(configOpts, prometheus.WithDurationBuckets(cfg.DurationBuckets))
	}
	if len(cfg.SizeBuckets) > 0 {
		configOpts = append(configOpts, prometheus.WithSizeBuckets(cfg.SizeBuckets))
	}
	configOpts = append(configOpts, prometheus.WithIgnoreMethods(cfg.IgnoreMethods...))
	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, prometheus.WithIgnorePatterns(patterns...))
	}

	return prometheus.ServerInterceptor(configOpts...), nil
}

// CreateTracingInterceptorFromConfig creates a tracing interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateTracingInterceptorFromConfig(
	cfg *config.GrpcInterTracingConfig,
	tracer tracing.Tracer,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if tracer == nil {
		tracer = tracing.Noop()
	}

	configOpts := []tracinginter.Option{
		tracinginter.WithLogger(f.Logger()),
		tracinginter.WithIgnoreMethods(cfg.IgnoreMethods...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		if len(patterns) > 0 {
			configOpts = append(configOpts, tracinginter.WithIgnorePatterns(patterns...))
		}
	}

	return tracinginter.ServerInterceptor(tracer, configOpts...), nil
}

// CreateRealIPInterceptorFromConfig creates a real IP extraction interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateRealIPInterceptorFromConfig(
	cfg *config.GrpcInterRealIpConfig,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	extractorOpts := []clientip.Option{
		clientip.WithLogger(f.Logger()),
		clientip.WithTrustedProxiesCount(cfg.TrustedProxiesCount),
	}

	if len(cfg.TrustedProxies) > 0 {
		proxies, err := parsePrefixes(cfg.TrustedProxies)
		if err != nil {
			return nil, f.WrapError(err, "failed to parse trusted proxies")
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedProxies(proxies...))
	}

	if len(cfg.TrustedPeers) > 0 {
		peers, err := parsePrefixes(cfg.TrustedPeers)
		if err != nil {
			return nil, f.WrapError(err, "failed to parse trusted peers")
		}
		extractorOpts = append(extractorOpts, clientip.WithTrustedPeers(peers...))
	}

	extractorOpts = append(extractorOpts, clientip.WithHeaders(cfg.Headers...))
	extractorOpts = append(extractorOpts, clientip.WithCacheSize(cfg.CacheSize))

	extractor, err := clientip.NewExtractor(extractorOpts...)
	if err != nil {
		return nil, f.WrapError(err, "failed to create real IP extractor")
	}

	configOpts := []realip.Option{realip.WithLogger(f.Logger())}
	return realip.ServerInterceptor(extractor, configOpts...), nil
}

// CreateRecoveryInterceptorFromConfig creates a panic recovery interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateRecoveryInterceptorFromConfig(
	cfg *config.GrpcInterRecoveryConfig,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	configOpts := []recovery.Option{
		recovery.WithLogger(f.Logger()),
	}

	configOpts = append(configOpts, recovery.WithIgnoreMethods(cfg.IgnoreMethods...))
	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, recovery.WithIgnorePatterns(patterns...))
	}

	return recovery.ServerInterceptor(configOpts...), nil
}

// CreateRequestIDInterceptorFromConfig creates a request ID interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
//
// Note: IgnoreMethods and IgnorePatterns from config are not yet applied
// as the requestid interceptor needs to be updated to support options.
func (f *Factory) CreateRequestIDInterceptorFromConfig(
	cfg *config.GrpcInterRequestIdConfig,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	var genOpts []sharedreqid.Option
	genOpts = slices.AppendIf(genOpts, cfg.GenerateIfMissing, sharedreqid.WithGenerateIfMissing())

	gen := sharedreqid.NewGenerator(genOpts...)

	return requestid.ServerInterceptor(gen), nil
}

// CreateLimiterInterceptorFromInterConfig creates a rate limiter interceptor from interceptor configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateLimiterInterceptorFromInterConfig(
	cfg *config.GrpcInterLimiterConfig,
	lim sharedlimiter.Limiter,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(lim, "limiter"); err != nil {
		return nil, err
	}

	configOpts := []limiter.Option{
		limiter.WithLogger(f.Logger()),
		limiter.WithFallbackBehavior(convertFallbackBehavior(cfg.FallbackBehavior)),
	}

	configOpts = append(configOpts, limiter.WithIgnoreMethods(cfg.IgnoreMethods...))
	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, limiter.WithIgnorePatterns(patterns...))
	}

	return limiter.ServerInterceptor(lim, configOpts...), nil
}

// CreateIdempotencyInterceptorFromInterConfig creates an idempotency interceptor from interceptor configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateIdempotencyInterceptorFromInterConfig(
	cfg *config.GrpcInterIdempotencyConfig,
	keeper idempotencydata.Idempotency,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(keeper, "idempotency keeper"); err != nil {
		return nil, err
	}

	configOpts := []idempotency.Option{
		idempotency.WithIdempotencyKeyHeader(cfg.IdempotencyKeyHeader),
		idempotency.WithIdempotencyKeyStatusMetadata(cfg.IdempotencyKeyStatusMetadata),
		idempotency.WithIdempotencyKeyEntityIdMetadata(cfg.IdempotencyKeyEntityIdMetadata),
		idempotency.WithFallbackBehavior(convertFallbackBehavior(cfg.FallbackBehavior)),
		idempotency.WithIgnoreMethods(cfg.IgnoreMethods...),
	}

	configOpts = slices.AppendIf(configOpts, cfg.EnforceMandatory, idempotency.WithEnforceMandatory())

	if len(cfg.IgnorePatterns) > 0 {
		configOpts = append(configOpts, idempotency.WithIgnorePatterns(compilePatterns(cfg.IgnorePatterns)...))
	}

	return idempotency.ServerInterceptor(keeper, configOpts...), nil
}

// CreateCacheInterceptorFromConfig creates a cache interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
func (f *Factory) CreateCacheInterceptorFromConfig(
	cfg *config.GrpcInterCacheConfig,
	cacher cache.Cacher,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(cacher, "cacher"); err != nil {
		return nil, err
	}

	opts := []cache.Option{
		cache.WithLogger(f.Logger()),
		cache.WithCacheHeaders(cfg.EnableCacheHeaders),
		cache.WithKeysPrefix(cfg.KeysPrefix),
		cache.WithCompressionPreset(convertCacheCompressionPreset(cfg.CompressionPreset)),
		cache.WithIgnoreMethods(cfg.IgnoreMethods...),
	}

	if len(cfg.KeyMetadata) > 0 {
		opts = append(opts, cache.WithMetadataKeys(cfg.KeyMetadata...))
	}

	if f.cacheMetadataProcessor != nil {
		opts = append(opts, cache.WithMetadataProcessor(f.cacheMetadataProcessor))
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		opts = append(opts, cache.WithIgnorePatterns(patterns...))
	}

	// Decision with separate TTLs
	opts = append(opts, cache.WithCacheDecision(
		cache.DefaultSuccessAndErrorDecision(cfg.SuccessTTL, cfg.ErrorTTL),
	))

	return cache.ServerInterceptor(cacher, opts...), nil
}

// CreateAuthInterceptorFromConfig creates an authentication interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
//
// Parameters:
//   - cfg: authentication interceptor configuration
//   - authFn: required authentication function for token validation
//   - clientAuth: optional client authentication handler for additional checks after initial auth
func (f *Factory) CreateAuthInterceptorFromConfig(
	cfg *config.GrpcInterAuthConfig,
	authFn auth.Auth,
	clientAuth auth.ClientAuth,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(authFn, "auth function"); err != nil {
		return nil, err
	}

	configOpts := []auth.Option{
		auth.WithLogger(f.Logger()),
		auth.WithAuthFn(authFn),
		auth.WithIgnoreMethods(cfg.IgnoreMethods...),
	}

	if clientAuth != nil {
		configOpts = append(configOpts, auth.WithClientAuth(clientAuth))
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, auth.WithIgnorePatterns(patterns...))
	}

	return auth.ServerInterceptor(configOpts...), nil
}

// CreateHealthInterceptorFromConfig creates a health check interceptor from configuration.
// Returns nil, nil if cfg is nil or disabled.
//
// Parameters:
//   - cfg: health interceptor configuration
//   - h: required health checker that reports service availability
func (f *Factory) CreateHealthInterceptorFromConfig(
	cfg *config.GrpcInterHealthConfig,
	h health.Health,
) (interceptors.ServerInterceptor, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, nil //nolint:nilnil
	}

	if err := f.RequireDependency(h, "health checker"); err != nil {
		return nil, err
	}

	configOpts := []health.Option{
		health.WithLogger(f.Logger()),
		health.WithIgnoreMethods(cfg.IgnoreMethods...),
	}

	if len(cfg.IgnorePatterns) > 0 {
		patterns := compilePatterns(cfg.IgnorePatterns)
		configOpts = append(configOpts, health.WithIgnorePatterns(patterns...))
	}

	return health.ServerInterceptor(h, configOpts...), nil
}

// convertCacheCompressionPreset converts config compression preset to interceptor preset.
func convertCacheCompressionPreset(p config.GrpcInterCacheCompressionPreset) cache.CompressionPreset {
	switch p {
	case config.GrpcInterCacheCompressionPresetNone:
		return cache.PresetNone
	case config.GrpcInterCacheCompressionPresetFast:
		return cache.PresetFast
	case config.GrpcInterCacheCompressionPresetBalanced:
		return cache.PresetBalanced
	case config.GrpcInterCacheCompressionPresetBest:
		return cache.PresetBest
	default:
		return cache.PresetBalanced
	}
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
