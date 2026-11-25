// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/tls"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/factory/transport/internal/factoryconv"
	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	router "github.com/altessa-s/go-atlas/transport/http/server/router"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
)

type serverSettings struct {
	cfg          *httpconfig.Config
	tlsProviders *tlsproviders.Providers
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

	// Listen address override
	listenAddress *string
}

// buildRouter creates the router. Returns (rootRouter, routerInterface) where
// rootRouter hosts pprof and metrics (the supplied router or the default root)
// and routerInterface is the router used by the server (possibly a subrouter).
func (b *serverSettings) buildRouter() (router.Router, router.Router) {
	if b.routerSet {
		return b.router, b.router
	}

	rt := std.New()

	var routerInterface router.Router = rt
	if b.cfg.RouterPrefix != "" {
		routerInterface = rt.PathPrefix(b.cfg.RouterPrefix).Subrouter()
	}

	return rt, routerInterface
}

// newServer maps HTTP server settings, applying explicit overrides after configuration defaults.
func (b *serverSettings) newServer(base *corefactory.Base, router router.Router, w server.ResponseWriter) (*server.Server, error) {
	opts := make([]server.Option, 0, 9)
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

	if w != nil {
		opts = append(opts, server.WithWriter(w))
	}

	addr := b.cfg.ListenAddress
	if b.listenAddress != nil {
		addr = *b.listenAddress
	}

	opts = append(opts, server.WithAddress(addr), server.WithLogger(base.Logger()))

	if b.tlsSet {
		// tlsSet && tlsConfig == nil → WithoutTLS(), no TLS config added
		opts = slices.AppendIf(opts, b.tlsConfig != nil, server.WithTLSConfig(b.tlsConfig))
	} else if b.cfg.TLS != nil {
		tlsConfig, err := factoryconv.ServerTLS(base, b.tlsProviders, b.cfg.TLS.Server)
		if err != nil {
			return nil, err
		}
		opts = append(opts, server.WithTLSConfig(tlsConfig))
	}

	return server.New(opts...)
}
