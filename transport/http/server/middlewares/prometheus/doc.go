// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prometheus provides middleware that records Prometheus metrics
// for HTTP server requests.
//
// The middleware tracks three core metrics and two optional size metrics:
//
//   - {prefix}server_requests_total (counter) - total requests by method and status
//   - {prefix}server_request_duration_seconds (histogram) - request latency
//   - {prefix}server_requests_in_flight (gauge) - concurrent requests
//   - {prefix}server_request_size_bytes (histogram, opt-in) - request body sizes
//   - {prefix}server_response_size_bytes (histogram, opt-in) - response body sizes
//
// Metrics are registered with a singleton pattern: the first call to [New]
// initializes and registers collectors; subsequent calls share those
// collectors but may apply different path-ignore filters.
//
// # Example
//
//	mw := prometheus.New(
//	    prometheus.WithNamespace("myapp"),
//	    prometheus.WithEnableSizeMetrics(),
//	)
//	handler := mw.Handler(yourHandler)
package prometheus
