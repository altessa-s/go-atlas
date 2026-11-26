// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas-core/encoding/keys"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrDescriptorConflict is returned when a metric name is redeclared with an
// incompatible descriptor (type, label names, help, or histogram buckets).
var ErrDescriptorConflict = fmt.Errorf("%w: metric descriptor conflict", coreerrs.ErrConflict)

// MetricType represents the type of a metric.
type MetricType int

const (
	// TypeCounter represents a counter metric.
	TypeCounter MetricType = iota
	// TypeGauge represents a gauge metric.
	TypeGauge
	// TypeHistogram represents a histogram metric.
	TypeHistogram
	// TypeTimer represents a duration metric stored as a histogram.
	TypeTimer
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
	case TypeTimer:
		return "timer"
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

	// Buckets contains histogram bucket boundaries (for histograms and timers).
	Buckets []float64
}

// Clone returns a deep copy of d with sorted LabelNames and cloned Buckets.
func (d *Desc) Clone() *Desc {
	if d == nil {
		return nil
	}
	cp := *d
	cp.LabelNames = sortedClone(d.LabelNames)
	cp.Buckets = slices.Clone(d.Buckets)
	return &cp
}

// Equal reports whether d and other describe the same metric name, type, help,
// sorted label names, and histogram buckets.
func (d *Desc) Equal(other *Desc) bool {
	if d == nil || other == nil {
		return d == other
	}
	return d.Name == other.Name &&
		d.Type == other.Type &&
		d.Help == other.Help &&
		slices.Equal(sortedClone(d.LabelNames), sortedClone(other.LabelNames)) &&
		slices.Equal(d.Buckets, other.Buckets)
}

func sortedClone(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(names))
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
}

// GaugeAdder is an optional [Adapter] capability for backends that support
// native additive gauge updates without requiring a local shadow value.
type GaugeAdder interface {
	// AddGauge adds delta (which may be negative) to the current gauge value.
	AddGauge(name string, labels map[string]string, delta float64)
}

// Flusher is an optional [Adapter] capability for writing buffered metrics.
type Flusher interface {
	// Flush ensures all pending metrics are written.
	// For pull-based systems like Prometheus, this may be a no-op.
	Flush() error
}

// Closer is an optional [Adapter] capability for releasing resources.
type Closer interface {
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

// BoundGaugeAdder is an optional [BoundGauge] capability for handles that
// support native additive updates.
type BoundGaugeAdder interface {
	BoundGauge

	// Add adds delta (which may be negative) to the bound gauge value.
	Add(delta float64)
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

type shadowGaugeSeries struct {
	mu    sync.Mutex
	value float64
}

// MultiAdapter wraps multiple [Adapter] instances to broadcast metric events.
// Not safe for concurrent modification after construction; concurrent
// method calls are safe.
type MultiAdapter struct {
	adapters        []Adapter
	hasShadowGauges bool

	descsMu sync.Mutex
	descs   map[string]*Desc

	gaugeSeries sync.Map // string -> *shadowGaugeSeries
}

// NewMultiAdapter creates an adapter that broadcasts to multiple adapters.
func NewMultiAdapter(children ...Adapter) *MultiAdapter {
	cloned := slices.Clone(children)
	var hasShadow bool
	for _, a := range cloned {
		if a == nil {
			continue
		}
		if _, ok := a.(GaugeAdder); !ok {
			hasShadow = true
			break
		}
	}
	return &MultiAdapter{
		adapters:        cloned,
		hasShadowGauges: hasShadow,
		descs:           make(map[string]*Desc),
	}
}

// Name implements Adapter.
func (m *MultiAdapter) Name() string {
	return "multi"
}

// Register implements Adapter.
// Broadcasts registration to all adapters, enforcing one descriptor per name
// and joining errors from any failing children.
func (m *MultiAdapter) Register(desc *Desc) error {
	if desc == nil {
		return fmt.Errorf("multi: nil descriptor")
	}
	norm := desc.Clone()

	m.descsMu.Lock()
	defer m.descsMu.Unlock()

	if existing, ok := m.descs[norm.Name]; ok {
		if !existing.Equal(norm) {
			return fmt.Errorf("multi: metric %q: %w", norm.Name, ErrDescriptorConflict)
		}
		return nil
	}

	var errs []error
	for _, a := range m.adapters {
		if a == nil {
			continue
		}
		if err := a.Register(norm); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	m.descs[norm.Name] = norm
	return nil
}

// RecordCounter implements Adapter.
// Broadcasts counter increment to all adapters.
func (m *MultiAdapter) RecordCounter(name string, labels map[string]string, delta float64) {
	for _, a := range m.adapters {
		if a != nil {
			a.RecordCounter(name, labels, delta)
		}
	}
}

// RecordGauge implements Adapter.
// Broadcasts gauge value to all adapters and updates shadow state for children
// that do not implement [GaugeAdder].
func (m *MultiAdapter) RecordGauge(name string, labels map[string]string, value float64) {
	if !m.hasShadowGauges {
		for _, a := range m.adapters {
			if a != nil {
				a.RecordGauge(name, labels, value)
			}
		}
		return
	}

	for _, a := range m.adapters {
		if _, ok := a.(GaugeAdder); ok {
			a.RecordGauge(name, labels, value)
		}
	}

	s := m.seriesFor(name, labels)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
	for _, a := range m.adapters {
		if a == nil {
			continue
		}
		if _, ok := a.(GaugeAdder); !ok {
			a.RecordGauge(name, labels, value)
		}
	}
}

// AddGauge implements [GaugeAdder].
// Delegates additive gauge updates directly to children implementing [GaugeAdder]
// and publishes the shadow absolute value to children without it.
func (m *MultiAdapter) AddGauge(name string, labels map[string]string, delta float64) {
	for _, a := range m.adapters {
		if ga, ok := a.(GaugeAdder); ok {
			ga.AddGauge(name, labels, delta)
		}
	}
	if !m.hasShadowGauges {
		return
	}

	s := m.seriesFor(name, labels)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value += delta
	val := s.value
	for _, a := range m.adapters {
		if a == nil {
			continue
		}
		if _, ok := a.(GaugeAdder); !ok {
			a.RecordGauge(name, labels, val)
		}
	}
}

// RecordHistogram implements Adapter.
// Broadcasts histogram observation to all adapters.
func (m *MultiAdapter) RecordHistogram(name string, labels map[string]string, value float64) {
	for _, a := range m.adapters {
		if a != nil {
			a.RecordHistogram(name, labels, value)
		}
	}
}

// Flush implements [Flusher].
// Flushes all participating adapters, joining any errors encountered.
func (m *MultiAdapter) Flush() error {
	var errs []error
	for _, a := range m.adapters {
		capable, ok := a.(Flusher)
		if !ok {
			continue
		}
		if err := capable.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close implements [Closer].
// Closes all participating adapters, joining any errors encountered.
func (m *MultiAdapter) Close() error {
	var errs []error
	for _, a := range m.adapters {
		capable, ok := a.(Closer)
		if !ok {
			continue
		}
		if err := capable.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *MultiAdapter) seriesFor(name string, labels map[string]string) *shadowGaugeSeries {
	key := multiSeriesKey(name, labels)
	if existing, ok := m.gaugeSeries.Load(key); ok {
		if s, ok := existing.(*shadowGaugeSeries); ok {
			return s
		}
	}
	actual, _ := m.gaugeSeries.LoadOrStore(key, &shadowGaugeSeries{})
	s, ok := actual.(*shadowGaugeSeries)
	if !ok {
		return &shadowGaugeSeries{}
	}
	return s
}

func multiSeriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return keys.Compose(name)
	}
	names := make([]string, 0, len(labels))
	for k := range labels {
		names = append(names, k)
	}
	slices.Sort(names)
	parts := make([]string, 0, 1+len(labels)*2)
	parts = append(parts, name)
	for _, k := range names {
		parts = append(parts, k, labels[k])
	}
	return keys.Compose(parts...)
}

// Ensure MultiAdapter implements required interfaces.
var (
	_ Adapter    = (*MultiAdapter)(nil)
	_ GaugeAdder = (*MultiAdapter)(nil)
	_ Flusher    = (*MultiAdapter)(nil)
	_ Closer     = (*MultiAdapter)(nil)
)
