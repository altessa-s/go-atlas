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

// buildRouter returns the router that serves requests and the router where
// endpoints are registered. A custom router serves both roles.
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
func (b *serverSettings) newServer(base *corefactory.Base, root, routes router.Router, w server.ResponseWriter) (*server.Server, error) {
	opts := make([]server.Option, 0, 9)
	opts = append(opts, server.WithRouter(root), server.WithRouteRegistrar(routes))

	if b.readTimeout != nil {
		opts = append(opts, server.WithReadTimeout(*b.readTimeout))
	} else {
		opts = append(opts, server.WithReadTimeout(b.cfg.ReadTimeout))
	}

	readHeaderTimeout, idleTimeout := b.readHeaderAndIdleTimeouts()
	opts = append(opts, server.WithReadHeaderTimeout(readHeaderTimeout), server.WithIdleTimeout(idleTimeout))

	if b.writeTimeout != nil {
		opts = append(opts, server.WithWriteTimeout(*b.writeTimeout))
	} else {
		opts = append(opts, server.WithWriteTimeout(b.cfg.WriteTimeout))
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

// readHeaderAndIdleTimeouts uses the HTTP config defaults for zero durations.
func (b *serverSettings) readHeaderAndIdleTimeouts() (time.Duration, time.Duration) {
	defaults := httpconfig.Default()
	readHeaderTimeout := b.cfg.ReadHeaderTimeout
	if b.readHeaderTimeout != nil {
		readHeaderTimeout = *b.readHeaderTimeout
	}
	if readHeaderTimeout == 0 {
		readHeaderTimeout = defaults.ReadHeaderTimeout
	}
	idleTimeout := b.cfg.IdleTimeout
	if b.idleTimeout != nil {
		idleTimeout = *b.idleTimeout
	}
	if idleTimeout == 0 {
		idleTimeout = defaults.IdleTimeout
	}
	return readHeaderTimeout, idleTimeout
}
