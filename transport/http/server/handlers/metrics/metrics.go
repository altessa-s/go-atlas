// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// Mount registers the Prometheus default-gatherer handler on r under
// `/metrics` and returns the mounted subrouter for further chaining.
//
// Compression is intentionally disabled to match the Prometheus
// scraper's expectations; pair it with the standard Prometheus exposition
// content type that promhttp.HandlerFor sets automatically.
func Mount(r router.Router) router.Router {
	metricsRouter := r.PathPrefix("/metrics").Subrouter()
	metricsRouter.Handle("/", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{DisableCompression: true},
	))

	return metricsRouter
}
