// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ServerBuilder) UseLogger(v *slog.Logger) *ServerBuilder {
	if v != nil {
		b.Base = corefactory.NewBase(v)
	}
	return b
}

// UseTlsProviders sets the TLS providers used for server TLS configuration.
func (b *ServerBuilder) UseTlsProviders(v *tlsproviders.Providers) *ServerBuilder {
	b.tlsProviders = v
	return b
}

// UseTracer sets the tracer used by the tracing middleware.
func (b *ServerBuilder) UseTracer(v tracing.Tracer) *ServerBuilder {
	b.tracer = v
	return b
}

// UseLimiter sets the rate limiter used by the limiter middleware.
func (b *ServerBuilder) UseLimiter(v sharedlimiter.Limiter) *ServerBuilder {
	b.limiter = v
	return b
}

// UseIdempotency sets the idempotency keeper used by the idempotency middleware.
func (b *ServerBuilder) UseIdempotency(v idempotencydata.Idempotency) *ServerBuilder {
	b.idempotency = v
	return b
}

// --- Router methods ---

// WithRouter sets a custom router. If not called, a Gorilla router is created.
func (b *ServerBuilder) WithRouter(r server.Router) *ServerBuilder {
	b.router = r
	b.routerSet = true
	return b
}

// WithRouterPrefix sets a path prefix for the default Gorilla router.
// Ignored when a custom router is set via [ServerBuilder.WithRouter].
func (b *ServerBuilder) WithRouterPrefix(prefix string) *ServerBuilder {
	// Prefix is applied in buildRouter; store in config.
	if b.cfg != nil {
		b.cfg.RouterPrefix = prefix
	}
	return b
}

// --- Timeout methods ---

// WithReadTimeout overrides the read timeout from config.
func (b *ServerBuilder) WithReadTimeout(d time.Duration) *ServerBuilder {
	b.readTimeout = &d
	return b
}

// WithWriteTimeout overrides the write timeout from config.
func (b *ServerBuilder) WithWriteTimeout(d time.Duration) *ServerBuilder {
	b.writeTimeout = &d
	return b
}

// WithIdleTimeout overrides the idle timeout from config.
func (b *ServerBuilder) WithIdleTimeout(d time.Duration) *ServerBuilder {
	b.idleTimeout = &d
	return b
}

// --- TLS methods ---

// WithTLSConfig sets an explicit TLS configuration, bypassing config-based TLS setup.
func (b *ServerBuilder) WithTLSConfig(tlsConfig *tls.Config) *ServerBuilder {
	b.tlsConfig = tlsConfig
	b.tlsSet = true
	return b
}

// WithoutTLS disables TLS even if TLS is configured in the config.
func (b *ServerBuilder) WithoutTLS() *ServerBuilder {
	b.tlsConfig = nil
	b.tlsSet = true
	return b
}

// --- Middleware disable methods ---

// WithoutBodyLimitMiddleware disables body limit middleware (useful after WithMiddlewares).
func (b *ServerBuilder) WithoutBodyLimitMiddleware() *ServerBuilder {
	return b.disableMiddleware("bodylimit")
}

// WithoutCorsMiddleware disables CORS middleware.
func (b *ServerBuilder) WithoutCorsMiddleware() *ServerBuilder {
	return b.disableMiddleware("cors")
}

// WithoutIdempotencyMiddleware disables idempotency middleware.
func (b *ServerBuilder) WithoutIdempotencyMiddleware() *ServerBuilder {
	return b.disableMiddleware("idempotency")
}

// WithoutIpAclMiddleware disables IP access control middleware.
func (b *ServerBuilder) WithoutIpAclMiddleware() *ServerBuilder {
	return b.disableMiddleware("ipacl")
}

// WithoutLimiterMiddleware disables rate limiter middleware.
func (b *ServerBuilder) WithoutLimiterMiddleware() *ServerBuilder {
	return b.disableMiddleware("limiter")
}

// WithoutLoggerMiddleware disables logger middleware.
func (b *ServerBuilder) WithoutLoggerMiddleware() *ServerBuilder {
	return b.disableMiddleware("logger")
}

// WithoutPrometheusMiddleware disables Prometheus middleware.
func (b *ServerBuilder) WithoutPrometheusMiddleware() *ServerBuilder {
	return b.disableMiddleware("prometheus")
}

// WithoutTracingMiddleware disables tracing middleware.
func (b *ServerBuilder) WithoutTracingMiddleware() *ServerBuilder {
	return b.disableMiddleware("tracing")
}

// WithoutRealIPMiddleware disables real IP middleware.
func (b *ServerBuilder) WithoutRealIPMiddleware() *ServerBuilder {
	return b.disableMiddleware("realip")
}

// WithoutRecoveryMiddleware disables recovery middleware.
func (b *ServerBuilder) WithoutRecoveryMiddleware() *ServerBuilder {
	return b.disableMiddleware("recovery")
}

// WithoutRequestIDMiddleware disables request ID middleware.
func (b *ServerBuilder) WithoutRequestIDMiddleware() *ServerBuilder {
	return b.disableMiddleware("requestid")
}

// WithoutSecurityHeadersMiddleware disables security headers middleware.
func (b *ServerBuilder) WithoutSecurityHeadersMiddleware() *ServerBuilder {
	return b.disableMiddleware("securityheaders")
}

func (b *ServerBuilder) disableMiddleware(name string) *ServerBuilder {
	if b.disabledMW == nil {
		b.disabledMW = make(map[string]struct{})
	}
	b.disabledMW[name] = struct{}{}
	return b
}

// --- Custom middleware methods ---

// WithMiddleware adds pre-built middleware instances. These are merged with
// config-based middleware, sorted by dependencies, and deduplicated.
func (b *ServerBuilder) WithMiddleware(mw ...middlewares.Middleware) *ServerBuilder {
	b.additionalMW = append(b.additionalMW, mw...)
	return b
}

// WithRawMiddleware adds raw middleware functions that bypass sorting
// and deduplication. They are registered after all sorted middleware.
func (b *ServerBuilder) WithRawMiddleware(mw ...server.Middleware) *ServerBuilder {
	b.rawMW = append(b.rawMW, mw...)
	return b
}

// --- Handler methods ---

// WithHandler adds custom handlers to the server.
func (b *ServerBuilder) WithHandler(h ...server.Handler) *ServerBuilder {
	b.handlers = append(b.handlers, h...)
	return b
}

// WithoutBuiltinHandlers disables all built-in handlers (ping, healthz, readyz, pprof, metrics).
func (b *ServerBuilder) WithoutBuiltinHandlers() *ServerBuilder {
	b.builtinEnabled = false
	b.pprofEnabled = false
	b.metricsEnabled = false
	return b
}

// WithoutPprof disables pprof handlers.
func (b *ServerBuilder) WithoutPprof() *ServerBuilder {
	b.pprofEnabled = false
	return b
}

// WithoutMetrics disables Prometheus metrics handler.
func (b *ServerBuilder) WithoutMetrics() *ServerBuilder {
	b.metricsEnabled = false
	return b
}

// WithInternalPrefix overrides the path prefix for built-in handlers
// (ping, healthz, readyz, pprof, metrics). Default is "/internal".
func (b *ServerBuilder) WithInternalPrefix(prefix string) *ServerBuilder {
	b.internalPrefix = prefix
	return b
}

// --- Address method ---

// WithListenAddress overrides the listen address from config.
func (b *ServerBuilder) WithListenAddress(addr string) *ServerBuilder {
	b.listenAddress = &addr
	return b
}
