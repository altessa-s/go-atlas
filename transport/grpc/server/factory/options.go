// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/security/tlsutils/providers"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ServerBuilder) UseLogger(v *slog.Logger) *ServerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ServerBuilder) UseDefaultLogger() *ServerBuilder {
	return b.UseLogger(slog.Default())
}

// UseTlsProviders sets the TLS providers used for server TLS configuration.
func (b *ServerBuilder) UseTlsProviders(v *tlsproviders.Providers) *ServerBuilder {
	b.tlsProviders = v
	return b
}

// UseCacheMetadataProcessor sets the metadata processor used by the cache interceptor.
func (b *ServerBuilder) UseCacheMetadataProcessor(v cache.MetadataProcessor) *ServerBuilder {
	b.cacheMetadataProcessor = v
	return b
}

// UseCacheMethodConfigs sets per-method cache configurations.
func (b *ServerBuilder) UseCacheMethodConfigs(v ...cache.MethodConfig) *ServerBuilder {
	b.cacheMethodConfigs = v
	return b
}

// UseTracer sets the tracer used by the tracing interceptor.
func (b *ServerBuilder) UseTracer(v tracing.Tracer) *ServerBuilder {
	b.tracer = v
	return b
}

// UseCollector sets the metrics collector used by the metrics interceptor.
// When unset, the metrics interceptor falls back to [metrics.Noop] and emits
// no samples — even if metrics is enabled in config. Wire a collector
// (typically `metrics.New(metrics.WithAdapter(...))`) at service init time
// to produce real metrics.
func (b *ServerBuilder) UseCollector(v metrics.Collector) *ServerBuilder {
	b.collector = v
	return b
}

// Collector returns the metrics collector configured via [UseCollector],
// or [metrics.Noop] when none was set.
func (b *ServerBuilder) Collector() metrics.Collector {
	if b.collector == nil {
		return metrics.Noop()
	}
	return b.collector
}

// UseLimiter sets the rate limiter used by the limiter interceptor.
func (b *ServerBuilder) UseLimiter(v sharedlimiter.Limiter) *ServerBuilder {
	b.limiter = v
	return b
}

// UseReasonCode sets the function used by the protovalidate interceptor to
// translate validation rule IDs into canonical, client-facing reason codes. The
// set of codes is part of a service's public error contract, so the service
// supplies its own [bufhelpers.ReasonCoder]; when unset, no code is emitted and
// consumers fall back to the generic gRPC-status reason.
func (b *ServerBuilder) UseReasonCode(v bufhelpers.ReasonCoder) *ServerBuilder {
	b.reasonCode = v
	return b
}

// UseReasonCodeResolver is the interface-based counterpart of [UseReasonCode]:
// it accepts any [bufhelpers.ReasonCodeResolver] (e.g. a service catalog type
// with a Resolve method) and adapts it to the validator's coder. A nil resolver
// is ignored.
func (b *ServerBuilder) UseReasonCodeResolver(v bufhelpers.ReasonCodeResolver) *ServerBuilder {
	if v != nil {
		b.reasonCode = v.Resolve
	}
	return b
}

// UseIdempotency sets the idempotency keeper used by the idempotency interceptor.
func (b *ServerBuilder) UseIdempotency(v idempotencydata.Idempotency) *ServerBuilder {
	b.idempotency = v
	return b
}

// UseCacher sets the cacher used by the cache interceptor.
func (b *ServerBuilder) UseCacher(v cache.Cacher) *ServerBuilder {
	b.cacher = v
	return b
}

// UseAuth sets the authentication function and optional client auth handler
// used by the auth interceptor.
func (b *ServerBuilder) UseAuth(authFn auth.Auth, clientAuth auth.ClientAuth) *ServerBuilder {
	b.authFn = authFn
	b.clientAuth = clientAuth
	return b
}

// UseHealthChecker sets the health checker used by the health interceptor.
func (b *ServerBuilder) UseHealthChecker(v health.Health) *ServerBuilder {
	b.healthChecker = v
	return b
}

// UseGeoResolver sets the geo resolver used by the geoacl interceptor.
func (b *ServerBuilder) UseGeoResolver(v geoacl.GeoResolver) *ServerBuilder {
	b.geoResolver = v
	return b
}

// UseErrStatusOptions adds code-supplied errstatus options — error rules,
// converters, sentinel errors — to the interceptor the configuration
// enables. They are applied after the options built from configuration, so
// a code-supplied finalizer replaces the configured one.
func (b *ServerBuilder) UseErrStatusOptions(opts ...errstatus.Option) *ServerBuilder {
	b.errStatusOptions = append(b.errStatusOptions, opts...)
	return b
}

// UseLoggerOptions adds code-supplied logger interceptor options — a payload
// redactor, a custom log func, extra ignore rules — to the interceptor the
// configuration enables. They are applied after the options built from
// configuration, so a code-supplied value replaces the configured one.
func (b *ServerBuilder) UseLoggerOptions(opts ...logger.Option) *ServerBuilder {
	b.loggerOptions = append(b.loggerOptions, opts...)
	return b
}

// UseFieldBehaviorOptions adds code-supplied field-behavior interceptor
// options — per-method kinds ([fieldbehavior.WithMethodKind]) for RPCs whose
// names do not follow the Create/Update naming convention,
// [fieldbehavior.WithSkipResponse] — to the interceptor
// [ServerBuilder.WithFieldBehaviorInterceptor] adds. They are applied after the
// builder's own options.
func (b *ServerBuilder) UseFieldBehaviorOptions(opts ...fieldbehavior.Option) *ServerBuilder {
	b.fieldBehaviorOptions = append(b.fieldBehaviorOptions, opts...)
	return b
}

// UseIdempotencyOptions adds code-supplied idempotency interceptor options —
// [idempotency.WithEntityIDExtractor], [idempotency.WithStatusCreator], a key
// format validator — to the interceptor the configuration enables. They are
// applied after the options built from configuration, so a code-supplied value
// replaces the configured one.
func (b *ServerBuilder) UseIdempotencyOptions(opts ...idempotency.Option) *ServerBuilder {
	b.idempotencyOptions = append(b.idempotencyOptions, opts...)
	return b
}

// UseFieldMaskOptions adds code-supplied field mask interceptor options —
// per-method kinds ([fieldmask.WithMethodKind]), update and read extractors for
// non-canonical request shapes — to the interceptor the configuration enables.
// They are applied after the options built from configuration.
func (b *ServerBuilder) UseFieldMaskOptions(opts ...fieldmask.Option) *ServerBuilder {
	b.fieldMaskOptions = append(b.fieldMaskOptions, opts...)
	return b
}

// UseServerOptions adds code-supplied server options — an injected listener
// ([grpcserver.WithListener], e.g. bufconn in tests), the startup check
// ([grpcserver.WithStartupVerification]) — applied after the options built
// from configuration. gRPC server options from configuration are kept;
// [grpcserver.WithGrpcOptions] here replaces them.
func (b *ServerBuilder) UseServerOptions(opts ...grpcserver.Option) *ServerBuilder {
	b.serverOptions = append(b.serverOptions, opts...)
	return b
}

// --- Custom interceptor methods ---

// WithInterceptor adds pre-built interceptor instances.
func (b *ServerBuilder) WithInterceptor(i ...interceptors.ServerInterceptor) *ServerBuilder {
	b.interceptors = append(b.interceptors, i...)
	return b
}
