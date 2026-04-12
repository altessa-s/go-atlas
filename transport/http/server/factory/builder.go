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

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/handler"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
)

// ServerBuilder assembles an HTTP [server.Server] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ServerBuilder.Build] time.
// The builder is not safe for concurrent use.
type ServerBuilder struct {
	corefactory.Base
	cfg  *config.Http
	errs []error

	// Dependencies
	tlsProviders *tlsproviders.Providers
	tracer       tracing.Tracer
	limiter      sharedlimiter.Limiter
	idempotency  idempotencydata.Idempotency
	geoResolver  geoacl.GeoResolver

	// Router
	router    server.Router
	routerSet bool

	// Timeouts (nil = use config value)
	readTimeout  *time.Duration
	writeTimeout *time.Duration
	idleTimeout  *time.Duration

	// TLS override
	tlsConfig *tls.Config
	tlsSet    bool

	// Middleware
	configMW     []middlewares.Middleware // from WithMiddlewares() / WithXMiddleware()
	additionalMW []middlewares.Middleware // from WithMiddleware()
	rawMW        []server.Middleware      // from WithRawMiddleware()
	disabledMW   map[string]struct{}      // from WithoutXMiddleware()

	// Handlers
	handlers       []server.Handler
	builtinEnabled bool
	pprofEnabled   *bool // nil = use config or default (true)
	metricsEnabled bool
	internalPrefix string

	// Listen address override
	listenAddress *string
}

// New creates a [ServerBuilder] for the given HTTP config.
// Config can be nil — the error surfaces at [ServerBuilder.Build] time.
func New(cfg *config.Http) *ServerBuilder {
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
		return nil, fmt.Errorf("configuration is required")
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
// rootRouter is the top-level Gorilla router (used for pprof/metrics subrouters)
// and routerInterface is the router used by the server (possibly a subrouter).
func (b *ServerBuilder) buildRouter() (*gorilla.Router, server.Router) {
	if b.routerSet {
		return nil, b.router
	}

	rt := gorilla.New()
	rt.Underlying().StrictSlash(true)

	var routerInterface server.Router = rt
	if b.cfg.RouterPrefix != "" {
		routerInterface = rt.PathPrefix(b.cfg.RouterPrefix).Subrouter()
	}

	return rt, routerInterface
}

// buildServerOptions builds server.Option slice.
func (b *ServerBuilder) buildServerOptions(router server.Router) []server.Option {
	opts := make([]server.Option, 0, 5)
	opts = append(opts, server.WithRouter(router))

	if b.readTimeout != nil {
		opts = append(opts, server.WithReadTimeout(*b.readTimeout))
	} else {
		opts = append(opts, server.WithReadTimeout(b.cfg.ReadTimeout))
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

	return opts
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
		if b.tlsConfig != nil {
			opts = append(opts, baseserver.WithTlsConfig(b.tlsConfig))
		}
		// tlsSet && tlsConfig == nil → WithoutTLS(), no TLS config added
	} else if b.cfg.TLS != nil {
		tlsConfig, err := b.buildServerTlsConfig()
		if err != nil {
			return nil, err
		}
		opts = append(opts, baseserver.WithTlsConfig(tlsConfig))
	}

	return opts, nil
}

// buildServerTlsConfig builds a TLS config from the builder's HTTP TLS configuration.
func (b *ServerBuilder) buildServerTlsConfig() (*tls.Config, error) {
	if err := b.RequireDependency(b.tlsProviders, "tls providers"); err != nil {
		return nil, err
	}

	cfg := b.cfg.TLS
	providerType := tlsproviders.ProviderType(cfg.ProviderType)
	provider, ok := b.tlsProviders.Get(providerType)
	if !ok {
		return nil, b.Errorf("tls provider %q not configured", cfg.ProviderType)
	}

	tlsConfig, err := provider.TLSConfig()
	if err != nil {
		return nil, b.WrapError(err, "failed to create TLS config from provider")
	}

	if cfg.MinTLSVersion == "1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	return tlsConfig, nil
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
		srv.RegisterMiddleware(slices.To(ordered, func(v middlewares.Middleware) server.Middleware {
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
func (b *ServerBuilder) registerBuiltinHandlers(srv *server.Server, rootRouter *gorilla.Router) {
	if b.builtinEnabled {
		srv.Handle(b.internalPrefix+"/ping", handler.Ping).Methods(http.MethodGet)
		srv.Handle(b.internalPrefix+"/healthz", handler.K8sHealtz).Methods(http.MethodGet)
		srv.Handle(b.internalPrefix+"/readyz", handler.K8sReadyz).Methods(http.MethodGet)
	}

	if rootRouter != nil {
		if b.resolvePprofEnabled() {
			handler.Pprof(rootRouter.PathPrefix(b.internalPrefix).Subrouter())
		}
		if b.metricsEnabled {
			handler.PrometheusMetrics(rootRouter.PathPrefix(b.internalPrefix).Subrouter())
		}
	}
}

// resolvePprofEnabled returns the effective pprof state:
// programmatic override > config > default (true).
func (b *ServerBuilder) resolvePprofEnabled() bool {
	if b.pprofEnabled != nil {
		return *b.pprofEnabled
	}
	if b.cfg != nil && b.cfg.Pprof != nil {
		return b.cfg.Pprof.Enabled
	}
	return true
}
