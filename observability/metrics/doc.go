// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metrics provides an abstract metrics collection system that is not tied
// to any specific format (Prometheus, StatsD, OpenTelemetry, etc.). Components use
// the abstract API, and export happens through adapters.
//
// # Architecture
//
// The package follows an adapter pattern where:
//   - Components depend on abstract interfaces (Counter, Gauge, Histogram, Timer)
//   - Collector manages metric creation and lifecycle
//   - Adapters translate abstract metrics to specific backends (Prometheus, StatsD, etc.)
//
// # Basic Usage
//
//	// Create a collector with Prometheus adapter
//	collector := metrics.New(
//	    metrics.WithNamespace("myapp"),
//	    metrics.WithAdapter(prometheusAdapter),
//	)
//
//	// Create metrics
//	requestsTotal := collector.MustCounter(metrics.MetricOpts{
//	    Name: "requests_total",
//	    Help: "Total number of requests",
//	    LabelNames: []string{"method", "status"},
//	})
//
//	// Use metrics
//	requestsTotal.WithLabels(metrics.Labels{"method": "GET", "status": "200"}).Inc()
//
// # Scoped Collector
//
// Components can use WithSubsystem to create a scoped collector with a subsystem prefix:
//
//	// Collector has namespace "myapp"
//	scopedCollector := collector.WithSubsystem("inprogress")
//
//	// Creates metric: myapp_inprogress_active_heartbeaters
//	gauge := scopedCollector.MustGauge(metrics.MetricOpts{
//	    Name: "active_heartbeaters",
//	    Help: "Current number of active heartbeaters",
//	})
//
// # Optional Metrics
//
// Components should accept metrics.Collector through options and default to Noop():
//
//	type options struct {
//	    metrics metrics.Collector
//	}
//
//	func newOptions(opts ...Option) *options {
//	    o := &options{
//	        metrics: metrics.Noop(), // Default to no-op
//	    }
//	    for _, opt := range opts {
//	        opt(o)
//	    }
//	    return o
//	}
//
// # Metric Naming Convention
//
// Full metric name is constructed as: {namespace}_{subsystem}_{name}
//
//   - namespace: Set once at Collector creation (e.g., "myapp")
//   - subsystem: Added via WithSubsystem (e.g., "broker", "cache")
//   - name: Defined in MetricOpts (e.g., "requests_total")
//
// Example: myapp_broker_requests_total
package metrics
