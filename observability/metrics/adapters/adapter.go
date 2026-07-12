// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import (
	"net/http"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// MetricType represents the type of a metric.
type MetricType int

const (
	// TypeCounter represents a counter metric.
	TypeCounter MetricType = iota
	// TypeGauge represents a gauge metric.
	TypeGauge
	// TypeHistogram represents a histogram metric.
	TypeHistogram
)

// String returns the string representation of the metric type.
func (t MetricType) String() string {
	switch t {
	case TypeCounter:
		return "counter"
	case TypeGauge:
		return "gauge"
	case TypeHistogram:
		return "histogram"
	default:
		return "unknown"
	}
}

// Desc describes a metric for registration with adapters.
type Desc struct {
	// Name is the full metric name including namespace and subsystem.
	Name string

	// Help is a human-readable description of the metric.
	Help string

	// Type is the metric type (counter, gauge, histogram).
	Type MetricType

	// LabelNames are the names of labels for this metric.
	LabelNames []string

	// Buckets contains histogram bucket boundaries (for histograms only).
	Buckets []float64
}

// Adapter is the interface that metrics backends must implement.
// It translates abstract metric operations to specific backend formats.
// Implementations must be safe for concurrent use.
type Adapter interface {
	// Name returns the adapter name (e.g., "prometheus", "statsd").
	Name() string

	// Register registers a metric with the backend.
	// This is called when a new metric is created.
	Register(desc *Desc) error

	// RecordCounter records a counter increment.
	// The delta should be added to the current counter value.
	RecordCounter(name string, labels map[string]string, delta float64)

	// RecordGauge records a gauge value.
	// The value should replace the current gauge value.
	RecordGauge(name string, labels map[string]string, value float64)

	// RecordHistogram records an observation in a histogram.
	RecordHistogram(name string, labels map[string]string, value float64)

	// Flush ensures all pending metrics are written.
	// For pull-based systems like Prometheus, this may be a no-op.
	Flush() error

	// Close releases any resources held by the adapter.
	Close() error
}

// HTTPHandler is an adapter that can expose metrics via HTTP.
// This is typically used by pull-based systems like Prometheus.
type HTTPHandler interface {
	Adapter

	// Handler returns an http.Handler that serves metrics.
	// For Prometheus, this would be the /metrics endpoint handler.
	Handler() http.Handler
}

// BoundCounter is a counter handle pre-resolved to one concrete label set.
type BoundCounter interface {
	// Add increments the bound counter by delta.
	Add(delta float64)
}

// BoundGauge is a gauge handle pre-resolved to one concrete label set.
type BoundGauge interface {
	// Set replaces the bound gauge value.
	Set(value float64)
}

// BoundHistogram is a histogram handle pre-resolved to one concrete label set.
type BoundHistogram interface {
	// Observe records a value in the bound histogram.
	Observe(value float64)
}

// Binder is an optional [Adapter] capability. Backends that can resolve a
// (metric name, label set) pair into a direct recording handle implement it,
// so the facade records through the handle without re-resolving the metric
// and re-hashing the label map on every observation. A Bind call returning
// false means the pair cannot be bound (unknown metric, invalid labels);
// callers must fall back to the corresponding RecordX method.
type Binder interface {
	// BindCounter resolves a counter child for the given label set.
	BindCounter(name string, labels map[string]string) (BoundCounter, bool)

	// BindGauge resolves a gauge child for the given label set.
	BindGauge(name string, labels map[string]string) (BoundGauge, bool)

	// BindHistogram resolves a histogram child for the given label set.
	BindHistogram(name string, labels map[string]string) (BoundHistogram, bool)
}

// MultiAdapter wraps multiple [Adapter] instances to broadcast metric events.
// Not safe for concurrent modification after construction; concurrent
// method calls are safe.
type MultiAdapter struct {
	adapters []Adapter
}

// NewMultiAdapter creates an adapter that broadcasts to multiple adapters.
func NewMultiAdapter(adapters ...Adapter) *MultiAdapter {
	return &MultiAdapter{adapters: adapters}
}

// Name implements Adapter.
func (m *MultiAdapter) Name() string {
	return "multi"
}

// Register implements Adapter.
// Broadcasts registration to all adapters.
func (m *MultiAdapter) Register(desc *Desc) error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.Register(desc); err != nil {
			return err
		}
	}
	return nil
}

// RecordCounter implements Adapter.
// Broadcasts counter increment to all adapters.
func (m *MultiAdapter) RecordCounter(name string, labels map[string]string, delta float64) {
	for a := range coreslices.Values(m.adapters) {
		a.RecordCounter(name, labels, delta)
	}
}

// RecordGauge implements Adapter.
// Broadcasts gauge value to all adapters.
func (m *MultiAdapter) RecordGauge(name string, labels map[string]string, value float64) {
	for a := range coreslices.Values(m.adapters) {
		a.RecordGauge(name, labels, value)
	}
}

// RecordHistogram implements Adapter.
// Broadcasts histogram observation to all adapters.
func (m *MultiAdapter) RecordHistogram(name string, labels map[string]string, value float64) {
	for a := range coreslices.Values(m.adapters) {
		a.RecordHistogram(name, labels, value)
	}
}

// Flush implements Adapter.
// Flushes all adapters, returning the first error encountered.
func (m *MultiAdapter) Flush() error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// Close implements Adapter.
// Closes all adapters, returning the first error encountered.
func (m *MultiAdapter) Close() error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Ensure MultiAdapter implements Adapter.
var _ Adapter = (*MultiAdapter)(nil)
