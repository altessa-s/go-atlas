// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"time"
)

// Collector is the interface for creating and managing metrics.
// Use [New] to create a concrete implementation or [Noop] for a no-op.
//
// All methods are safe for concurrent use. After [Collector.Shutdown],
// metric operations become no-ops.
type Collector interface {
	// Counter creates or retrieves a counter metric.
	Counter(opts MetricOpts) Counter

	// Gauge creates or retrieves a gauge metric.
	Gauge(opts MetricOpts) Gauge

	// Histogram creates or retrieves a histogram metric.
	Histogram(opts HistogramOpts) Histogram

	// Timer creates or retrieves a timer metric (histogram-based).
	Timer(opts HistogramOpts) Timer

	// MustCounter creates a counter, panicking on error.
	MustCounter(opts MetricOpts) Counter

	// MustGauge creates a gauge, panicking on error.
	MustGauge(opts MetricOpts) Gauge

	// MustHistogram creates a histogram, panicking on error.
	MustHistogram(opts HistogramOpts) Histogram

	// MustTimer creates a timer, panicking on error.
	MustTimer(opts HistogramOpts) Timer

	// WithSubsystem returns a Collector with the given subsystem.
	// The subsystem is added between namespace and metric name:
	// {namespace}_{subsystem}_{name}
	WithSubsystem(subsystem string) Collector

	// Shutdown shuts down the collector and all adapters.
	// After Shutdown is called, metric operations become no-ops.
	// It should be called when the application exits.
	// Consistent with TracerProvider.Shutdown.
	Shutdown(ctx context.Context) error

	// ForceFlush forces an immediate flush of all buffered metrics.
	// For pull-based systems like Prometheus, this may be a no-op.
	// Consistent with TracerProvider.ForceFlush.
	ForceFlush(ctx context.Context) error
}

// Counter is a metric that can only increase or be reset to zero.
// Obtain via [Collector.Counter] or [Collector.MustCounter].
// All methods are safe for concurrent use.
type Counter interface {
	// Inc increments the counter by 1.
	Inc()

	// Add increments the counter by the given delta.
	// Delta must be non-negative.
	Add(delta float64)

	// WithLabels returns a Counter with the specified labels applied.
	// The returned Counter shares the same underlying metric but with
	// the label values set.
	WithLabels(labels Labels) Counter
}

// Gauge is a metric that can increase and decrease.
// Obtain via [Collector.Gauge] or [Collector.MustGauge].
// All methods are safe for concurrent use.
type Gauge interface {
	// Set sets the gauge to the given value.
	Set(value float64)

	// Inc increments the gauge by 1.
	Inc()

	// Dec decrements the gauge by 1.
	Dec()

	// Add adds the given delta to the gauge.
	// Delta can be negative to decrease the gauge.
	Add(delta float64)

	// Sub subtracts the given delta from the gauge.
	Sub(delta float64)

	// WithLabels returns a Gauge with the specified labels applied.
	WithLabels(labels Labels) Gauge
}

// Histogram tracks the distribution of values across configured bucket boundaries.
// Obtain via [Collector.Histogram] or [Collector.MustHistogram].
// All methods are safe for concurrent use.
type Histogram interface {
	// Observe records a value in the histogram.
	Observe(value float64)

	// WithLabels returns a Histogram with the specified labels applied.
	WithLabels(labels Labels) Histogram
}

// Timer is a specialized metric for measuring durations.
// Internally it uses a [Histogram] to track the distribution of durations.
// Obtain via [Collector.Timer] or [Collector.MustTimer].
// All methods are safe for concurrent use.
type Timer interface {
	// Start begins timing and returns a stop function.
	// Call the returned function to record the duration.
	//
	// Example:
	//   stop := timer.Start()
	//   defer stop()
	//   // ... do work ...
	Start() (stop func())

	// ObserveDuration records a duration directly.
	// Use this when you already have a computed duration.
	ObserveDuration(d time.Duration)

	// WithLabels returns a Timer with the specified labels applied.
	WithLabels(labels Labels) Timer
}
