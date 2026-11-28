// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	healthh "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
	metricsh "github.com/altessa-s/go-atlas/transport/http/server/handlers/metrics"
	pingh "github.com/altessa-s/go-atlas/transport/http/server/handlers/ping"
	pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
	router "github.com/altessa-s/go-atlas/transport/http/server/router"
)

type endpoints struct {
	// Handlers
	handlers       []server.Handler
	builtinEnabled bool
	pprofEnabled   *bool // nil = use config or default (false: pprof is opt-in)
	metricsEnabled bool
	metricsHandler http.Handler
	healthReporter healthh.HealthReporter
	internalPrefix string
}

// registerBuiltinHandlers registers built-in HTTP handlers (ping, healthz, readyz, pprof, metrics). With a
// health reporter, readyz answers its overall status and healthz/details lists its services; liveness stays static,
// so a failing dependency never restarts the process.
func (b *endpoints) registerBuiltinHandlers(srv *server.Server, routes router.Router, cfg *httpconfig.Pprof) {
	if b.builtinEnabled {
		srv.Handle(b.internalPrefix+"/ping", pingh.Handler).Methods(http.MethodGet)
		srv.Handle(b.internalPrefix+"/healthz", healthh.K8sHealtz).Methods(http.MethodGet)
		if b.healthReporter != nil {
			srv.Handle(b.internalPrefix+"/readyz", healthh.Readyz(b.healthReporter)).Methods(http.MethodGet)
			srv.Handle(b.internalPrefix+"/healthz/details", healthh.Detailed(b.healthReporter)).Methods(http.MethodGet)
		} else {
			srv.Handle(b.internalPrefix+"/readyz", healthh.K8sReadyz).Methods(http.MethodGet)
		}
	}

	pprof := b.resolvePprofEnabled(cfg)
	metrics := b.metricsEnabled && b.metricsHandler != nil
	if routes == nil || (!pprof && !metrics) {
		return
	}
	internal := routes.PathPrefix(b.internalPrefix).Subrouter()
	if pprof {
		pprofh.Mount(internal)
	}
	if metrics {
		metricsh.Mount(internal, b.metricsHandler)
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
func (b *endpoints) resolvePprofEnabled(cfg *httpconfig.Pprof) bool {
	if b.pprofEnabled != nil {
		return *b.pprofEnabled
	}
	if cfg != nil {
		return cfg.Enabled
	}
	return false
}
