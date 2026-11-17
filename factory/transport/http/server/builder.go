// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
	"github.com/altessa-s/go-atlas/transport/policy/geoacl"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	baseserver "github.com/altessa-s/go-atlas/internal/transport/server"
	idempotencydata "github.com/altessa-s/go-atlas/reliability/idempotency"
	healthh "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
	metricsh "github.com/altessa-s/go-atlas/transport/http/server/handlers/metrics"
	pingh "github.com/altessa-s/go-atlas/transport/http/server/handlers/ping"
	pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
	router "github.com/altessa-s/go-atlas/transport/http/server/router"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

// ServerBuilder assembles an HTTP [server.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	cfg  *httpconfig.Config
	errs []error

	// Dependencies
	tlsProviders *tlsproviders.Providers
	tracer       tracing.Tracer
	collector    metrics.Meter
	limiter      sharedlimiter.Limiter
	idempotency  idempotencydata.Idempotency
	geoResolver  geoacl.GeoResolver

	// Router
	router    router.Router
	routerSet bool

	// Timeouts (nil = use config value)
	readTimeout       *time.Duration
	readHeaderTimeout *time.Duration
	writeTimeout      *time.Duration
	idleTimeout       *time.Duration

	// TLS override
	tlsConfig *tls.Config
	tlsSet    bool

	// Middleware
	configMW     []middlewares.Middleware // from WithMiddlewares() / WithXMiddleware()
	additionalMW []middlewares.Middleware // from WithMiddleware()
	rawMW        []router.Middleware      // from WithRawMiddleware()
	disabledMW   map[string]struct{}      // from WithoutXMiddleware()

	// Handlers
	handlers       []server.Handler
	builtinEnabled bool
	pprofEnabled   *bool // nil = use config or default (false: pprof is opt-in)
	metricsEnabled bool
	metricsHandler http.Handler
	internalPrefix string

	// Listen address override
	listenAddress *string
}

// New creates a [ServerBuilder] for the given HTTP config.
// Config can be nil — the error surfaces at [ServerBuilder.Build] time.
func New(cfg *httpconfig.Config) *ServerBuilder {
	return &ServerBuilder{
		Base:           corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:            cfg,
		builtinEnabled: true,
		metricsEnabled: true,
		internalPrefix: "/internal",
	}
}

// --- Build ---

// Build assembles the server. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *ServerBuilder) Build() (*server.Server, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	rootRouter, routerInterface := b.buildRouter()

	opts := b.buildServerOptions(routerInterface)
	baseOpts, err := b.buildBaseOptions()
	if err != nil {
		return nil, err
	}
	opts = append(opts, server.WithBaseOptions(baseOpts...))

	srv, err := server.New(opts...)
	if err != nil {
		return nil, err
	}

	// Collect, filter, sort and register middleware.
	if err := b.registerMiddleware(srv); err != nil {
		return nil, coreerrs.WrapOperation(err, "register HTTP middlewares")
	}

	// Register built-in handlers.
	b.registerBuiltinHandlers(srv, rootRouter)

	// Register custom handlers.
	srv.RegisterHandlers(b.handlers...)

	return srv, nil
}

// buildRouter creates the router. Returns (rootRouter, routerInterface) where
// rootRouter is the top-level [std.Router] (used for pprof/metrics subrouters)
// and routerInterface is the router used by the server (possibly a subrouter).
func (b *ServerBuilder) buildRouter() (*std.Router, router.Router) {
	if b.routerSet {
		return nil, b.router
	}

	rt := std.New()

	var routerInterface router.Router = rt
	if b.cfg.RouterPrefix != "" {
		routerInterface = rt.PathPrefix(b.cfg.RouterPrefix).Subrouter()
	}

	return rt, routerInterface
}

// buildServerOptions builds server.Option slice.
func (b *ServerBuilder) buildServerOptions(router router.Router) []server.Option {
	opts := make([]server.Option, 0, 5)
	opts = append(opts, server.WithRouter(router))

	if b.readTimeout != nil {
		opts = append(opts, server.WithReadTimeout(*b.readTimeout))
	} else {
		opts = append(opts, server.WithReadTimeout(b.cfg.ReadTimeout))
	}

	if b.readHeaderTimeout != nil {
		opts = append(opts, server.WithReadHeaderTimeout(*b.readHeaderTimeout))
	} else if b.cfg.ReadHeaderTimeout > 0 {
		// Honor an explicit config value, but fall back to the server
		// package's own default (DefaultReadHeaderTimeout) for older
		// configs that predate this field. Falling through silently
		// would leave the server with ReadHeaderTimeout=0, which is
		// the unsafe pre-fix behavior we're closing.
		opts = append(opts, server.WithReadHeaderTimeout(b.cfg.ReadHeaderTimeout))
	}

	if b.writeTimeout != nil {
		opts = append(opts, server.WithWriteTimeout(*b.writeTimeout))
	} else {
		opts = append(opts, server.WithWriteTimeout(b.cfg.WriteTimeout))
	}

	if b.idleTimeout != nil {
		opts = append(opts, server.WithIdleTimeout(*b.idleTimeout))
	} else {
		opts = append(opts, server.WithIdleTimeout(b.cfg.IdleTimeout))
	}

	if w := b.bodyLimitWriter(); w != nil {
		opts = append(opts, server.WithWriter(w))
	}

	return opts
}

// bodyLimitWriter applies an explicit reader cap, or aligns it with an installed
// config bodylimit middleware. Disabled/uninstalled middleware cannot raise it.
func (b *ServerBuilder) bodyLimitWriter() server.ResponseWriter {
	if b.cfg.MaxBodySize != 0 {
		return writer.New(writer.WithMaxBodySize(b.cfg.MaxBodySize))
	}
	cfg := b.middlewaresCfg()
	if cfg == nil || !cfg.BodyLimit.IsEnabled() || cfg.BodyLimit.MaxSize <= 0 {
		return nil
	}
	if _, disabled := b.disabledMW[bodylimit.Name()]; disabled {
		return nil
	}
	for _, mw := range b.configMW {
		if mw.Name() == bodylimit.Name() {
			return writer.New(writer.WithMaxBodySize(cfg.BodyLimit.MaxSize))
		}
	}
	return nil
}

// buildBaseOptions builds baseserver.Option slice.
func (b *ServerBuilder) buildBaseOptions() ([]baseserver.Option, error) {
	addr := b.cfg.ListenAddress
	if b.listenAddress != nil {
		addr = *b.listenAddress
	}

	opts := []baseserver.Option{
		baseserver.WithAddress(addr),
		baseserver.WithLogger(b.Logger()),
	}

	if b.tlsSet {
		// tlsSet && tlsConfig == nil → WithoutTLS(), no TLS config added
		opts = slices.AppendIf(opts, b.tlsConfig != nil, baseserver.WithTLSConfig(b.tlsConfig))
	} else if b.cfg.TLS != nil {
		tlsConfig, err := b.buildServerTlsConfig()
		if err != nil {
			return nil, err
		}
		opts = append(opts, baseserver.WithTLSConfig(tlsConfig))
	}

	return opts, nil
}

// buildServerTlsConfig builds a TLS config from the builder's HTTP TLS configuration.
func (b *ServerBuilder) buildServerTlsConfig() (*tls.Config, error) {
	if err := b.RequireDependency(b.tlsProviders, "tls providers"); err != nil {
		return nil, err
	}

	cfg := b.cfg.TLS
	providerType := tlsproviders.ProviderType(cfg.Provider)
	provider, ok := b.tlsProviders.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("tls provider %q not configured", cfg.Provider)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, b.WrapError(err, "failed to create TLS config from provider")
	}

	if err := tlsutils.ApplyMinTLSVersion(tlsConfig, cfg.MinTLSVersion); err != nil {
		return nil, b.WrapError(err, "tls.minVersion")
	}

	return tlsConfig, nil
}

// pinRecoveryOutermost moves the recovery middleware (if present) to
// position 0 of the ordered slice so it wraps every other middleware
// in the chain. Topological ordering otherwise places requestid before
// recovery (recovery declares requestid as a dependency), which leaves
// a panic inside requestid uncaught. Recovery still observes the
// request ID from context — requestid runs INSIDE recovery and
// populates the context before any handler panic — so the dependency
// contract is preserved in spirit.
func pinRecoveryOutermost(ordered []middlewares.Middleware) []middlewares.Middleware {
	for i, mw := range ordered {
		if mw == nil || mw.Name() != recovery.Name() {
			continue
		}
		if i == 0 {
			return ordered
		}
		// Move ordered[i] to index 0 while preserving the relative
		// order of the rest. copy() handles overlapping ranges.
		rec := ordered[i]
		copy(ordered[1:i+1], ordered[0:i])
		ordered[0] = rec
		return ordered
	}
	return ordered
}

// registerMiddleware collects all middleware, filters by disabledMW, sorts,
// deduplicates, and registers them on the server.
func (b *ServerBuilder) registerMiddleware(srv *server.Server) error {
	// Merge config-based and additional middleware.
	allMW := make([]middlewares.Middleware, 0, len(b.configMW)+len(b.additionalMW))
	allMW = append(allMW, b.configMW...)
	allMW = append(allMW, b.additionalMW...)

	// Filter out disabled middleware.
	if len(b.disabledMW) > 0 {
		filtered := make([]middlewares.Middleware, 0, len(allMW))
		for _, mw := range allMW {
			if mw == nil {
				continue
			}
			if _, disabled := b.disabledMW[mw.Name()]; !disabled {
				filtered = append(filtered, mw)
			}
		}
		allMW = filtered
	}

	// Sort by dependencies and deduplicate.
	if len(allMW) > 0 {
		ordered, err := middlewares.OrderMiddlewaresWithDedupe(allMW)
		if err != nil {
			return err
		}
		// Force the recovery middleware (if present) to be outermost,
		// regardless of where the topological sort placed it. Recovery
		// declares requestid as a dependency so the sort puts requestid
		// outside recovery — but that means a panic inside requestid
		// (or any non-middleware path the sort placed before recovery)
		// would crash the process instead of being caught. Recovery's
		// dependency is "I read requestid from context if available",
		// which still works when requestid runs INSIDE recovery: the
		// inner middleware populates the context BEFORE the handler
		// panics, and recovery's defer fires with the populated value.
		ordered = pinRecoveryOutermost(ordered)
		srv.RegisterMiddleware(slices.To(ordered, func(v middlewares.Middleware) router.Middleware {
			return v.Handler
		})...)
	}

	// Register raw middleware (bypasses sorting).
	if len(b.rawMW) > 0 {
		srv.RegisterMiddleware(b.rawMW...)
	}

	return nil
}

// registerBuiltinHandlers registers built-in HTTP handlers (ping, healthz, readyz, pprof, metrics).
func (b *ServerBuilder) registerBuiltinHandlers(srv *server.Server, rootRouter *std.Router) {
	if b.builtinEnabled {
		srv.Handle(b.internalPrefix+"/ping", pingh.Handler).Methods(http.MethodGet)
		srv.Handle(b.internalPrefix+"/healthz", healthh.K8sHealtz).Methods(http.MethodGet)
		srv.Handle(b.internalPrefix+"/readyz", healthh.K8sReadyz).Methods(http.MethodGet)
	}

	if rootRouter != nil {
		if b.resolvePprofEnabled() {
			pprofh.Mount(rootRouter.PathPrefix(b.internalPrefix).Subrouter())
		}
		if b.metricsEnabled && b.metricsHandler != nil {
			metricsh.Mount(rootRouter.PathPrefix(b.internalPrefix).Subrouter(), b.metricsHandler)
		}
	}
}

// resolvePprofEnabled returns the effective pprof state:
// programmatic override > config > default (false).
//
// pprof exposes heap, goroutine and CPU profiles that can leak runtime
// internals and enable cheap denial-of-service via expensive profile
// collection. It must therefore be opt-in: enable it explicitly with
// [ServerBuilder.WithPprof] or by setting `pprof.enabled: true` in the
// HTTP server configuration.
func (b *ServerBuilder) resolvePprofEnabled() bool {
	if b.pprofEnabled != nil {
		return *b.pprofEnabled
	}
	if b.cfg != nil && b.cfg.Pprof != nil {
		return b.cfg.Pprof.Enabled
	}
	return false
}
