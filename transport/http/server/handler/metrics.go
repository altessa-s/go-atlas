// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// PrometheusMetrics registers a Prometheus metrics handler on the provided router.
// Returns a subrouter mounted at /metrics.
func PrometheusMetrics(r router.Router) router.Router {
	metricsRouter := r.PathPrefix("/metrics").Subrouter()
	metricsRouter.Handle("/", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{DisableCompression: true},
	))

	return metricsRouter
}
