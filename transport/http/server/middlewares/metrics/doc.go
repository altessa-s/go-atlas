// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metrics provides middleware that records HTTP server metrics via
// the [observability/metrics.Collector] abstraction.
//
// The middleware tracks three core metrics and two optional size metrics
// (names shown after [Collector.WithSubsystem] is applied; default subsystem
// is "http"):
//
//   - {service}_http_server_requests_total — total requests, labels: method, status
//   - {service}_http_server_request_duration_seconds — request latency histogram
//   - {service}_http_server_requests_in_flight — concurrent requests (gauge)
//   - {service}_http_server_request_size_bytes — request body sizes (opt-in)
//   - {service}_http_server_response_size_bytes — response body sizes (opt-in)
//
// Each call to [New] constructs a fresh middleware wired to the provided
// [metrics.Collector]. The underlying adapter deduplicates metric
// registrations by name, so reusing the same collector across multiple calls
// with the same subsystem shares the same metric vectors. To emit metrics
// for two unrelated upstreams against a single registry, pass distinct
// subsystems via [WithMetricsSubsystem].
//
// # Example
//
//	mw := metrics.New(
//	    metrics.WithCollector(coll),
//	    metrics.WithMetricsSubsystem("api"),
//	    metrics.WithEnableSizeMetrics(),
//	)
//	handler := mw.Handler(yourHandler)
package metrics
