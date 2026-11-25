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
//   - Meter creates metrics; it is the narrow handle libraries accept
//   - Collector is a Meter that also owns the lifecycle (Shutdown, ForceFlush)
//     and stays with the application's composition root
//   - Adapters translate abstract metrics to specific backends (Prometheus, StatsD, etc.)
//
// # Basic Usage
//
//	// Create a collector with Prometheus adapter
//	collector := metrics.New(
//	    metrics.WithServiceName("myapp"),
//	    metrics.WithAdapter(prometheusAdapter),
//	)
//
//	// Create metrics
//	requestsTotal := metrics.MustCounter(collector, metrics.MetricOpts{
//	    Name: "requests_total",
//	    Help: "Total number of requests",
//	    LabelNames: []string{"method", "status"},
//	})
//
//	// Use metrics
//	requestsTotal.WithLabels(metrics.Labels{"method": "GET", "status": "200"}).Inc()
//
// # Scoped Meter
//
// Components can use WithSubsystem to create a scoped [Meter] with a subsystem prefix.
// The scoped Meter shares the parent's instruments but does not expose its lifecycle:
//
//	// Collector has namespace "myapp"
//	scoped := collector.WithSubsystem("inprogress")
//
//	// Creates metric: myapp_inprogress_active_heartbeaters
//	gauge := metrics.MustGauge(scoped, metrics.MetricOpts{
//	    Name: "active_heartbeaters",
//	    Help: "Current number of active heartbeaters",
//	})
//
// The Must helpers validate options and panic on invalid input, including with [Noop].
// [MustMeter] provides the same helpers as methods for repeated registrations.
//
// # Optional Metrics
//
// Components should accept metrics.Meter through options and default to Noop().
// Only the composition root holds the [Collector] and calls Shutdown on exit:
//
//	type options struct {
//	    metrics metrics.Meter
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
