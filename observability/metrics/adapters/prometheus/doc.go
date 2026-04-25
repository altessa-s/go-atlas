// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prometheus provides a Prometheus [adapters.Adapter] for the metrics system.
// It translates abstract metric operations to prometheus client_golang metrics
// and implements [adapters.HTTPHandler] for the /metrics endpoint.
//
// Use [WithRegistry] to provide a custom prometheus.Registry instead of the default.
//
// # Example
//
//	adapter := prometheus.New()
//	collector := metrics.New(
//	    metrics.WithNamespace("myapp"),
//	    metrics.WithAdapter(adapter),
//	)
//
//	// Expose metrics via HTTP
//	http.Handle("/metrics", adapter.Handler())
package prometheus
