// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/security/tlsutils/providers"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
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

// UseLimiter sets the rate limiter used by the limiter interceptor.
func (b *ServerBuilder) UseLimiter(v sharedlimiter.Limiter) *ServerBuilder {
	b.limiter = v
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

// --- Custom interceptor methods ---

// WithInterceptor adds pre-built interceptor instances.
func (b *ServerBuilder) WithInterceptor(i ...interceptors.ServerInterceptor) *ServerBuilder {
	b.interceptors = append(b.interceptors, i...)
	return b
}
