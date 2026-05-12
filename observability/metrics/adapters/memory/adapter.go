// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory [adapters.Adapter] implementation
// for use in tests. Metric values are accumulated in goroutine-safe maps
// and exposed via lookup methods, so tests can assert on emitted samples
// without standing up a Prometheus registry or any other backend.
package memory

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// Adapter implements [adapters.Adapter] in process memory. It tracks
// counter sums, latest gauge values, and histogram observations
// (count + sum) keyed by metric name and label values. All public
// methods are safe for concurrent use.
type Adapter struct {
	mu         sync.RWMutex
	registered map[string]*adapters.Desc
	counters   map[string]map[string]float64
	gauges     map[string]map[string]float64
	histograms map[string]map[string]*histogramSeries
}

// histogramSeries accumulates observations for one label combination.
type histogramSeries struct {
	count uint64
	sum   float64
}

// New creates a fresh in-memory adapter with no registered metrics.
func New() *Adapter {
	return &Adapter{
		registered: make(map[string]*adapters.Desc),
		counters:   make(map[string]map[string]float64),
		gauges:     make(map[string]map[string]float64),
		histograms: make(map[string]map[string]*histogramSeries),
	}
}

// Name implements [adapters.Adapter].
func (a *Adapter) Name() string { return "memory" }

// Register implements [adapters.Adapter]. Re-registration of an
// already-known name is a no-op so repeated [metrics.Collector.MustCounter]
// calls with the same name behave like the Prometheus adapter.
func (a *Adapter) Register(desc *adapters.Desc) error {
	if desc == nil {
		return fmt.Errorf("memory: nil descriptor")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.registered[desc.Name]; ok {
		return nil
	}
	// Defensive copy of LabelNames + Buckets so callers can mutate their slices.
	copied := *desc
	copied.LabelNames = slices.Clone(desc.LabelNames)
	copied.Buckets = slices.Clone(desc.Buckets)
	a.registered[desc.Name] = &copied
	switch desc.Type {
	case adapters.TypeCounter:
		a.counters[desc.Name] = make(map[string]float64)
	case adapters.TypeGauge:
		a.gauges[desc.Name] = make(map[string]float64)
	case adapters.TypeHistogram:
		a.histograms[desc.Name] = make(map[string]*histogramSeries)
	}
	return nil
}

// RecordCounter implements [adapters.Adapter].
func (a *Adapter) RecordCounter(name string, labels map[string]string, delta float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	series, ok := a.counters[name]
	if !ok {
		return
	}
	series[labelKey(labels)] += delta
}

// RecordGauge implements [adapters.Adapter].
func (a *Adapter) RecordGauge(name string, labels map[string]string, value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	series, ok := a.gauges[name]
	if !ok {
		return
	}
	series[labelKey(labels)] = value
}

// RecordHistogram implements [adapters.Adapter].
func (a *Adapter) RecordHistogram(name string, labels map[string]string, value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	series, ok := a.histograms[name]
	if !ok {
		return
	}
	key := labelKey(labels)
	entry := series[key]
	if entry == nil {
		entry = &histogramSeries{}
		series[key] = entry
	}
	entry.count++
	entry.sum += value
}

// Flush implements [adapters.Adapter]. No-op for in-memory storage.
func (a *Adapter) Flush() error { return nil }

// Close implements [adapters.Adapter]. Releases all accumulated samples.
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.registered)
	clear(a.counters)
	clear(a.gauges)
	clear(a.histograms)
	return nil
}

// CounterValue returns the accumulated value of the named counter for
// the given label combination. Returns 0 when the metric is unknown or
// no observation matches the labels.
func (a *Adapter) CounterValue(name string, labels map[string]string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	series, ok := a.counters[name]
	if !ok {
		return 0
	}
	return series[labelKey(labels)]
}

// GaugeValue returns the most recently set value of the named gauge for
// the given label combination. Returns 0 when the metric is unknown or
// no observation matches the labels.
func (a *Adapter) GaugeValue(name string, labels map[string]string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	series, ok := a.gauges[name]
	if !ok {
		return 0
	}
	return series[labelKey(labels)]
}

// HistogramCount returns the number of observations recorded for the
// named histogram and label combination.
func (a *Adapter) HistogramCount(name string, labels map[string]string) uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	series, ok := a.histograms[name]
	if !ok {
		return 0
	}
	entry, ok := series[labelKey(labels)]
	if !ok {
		return 0
	}
	return entry.count
}

// HistogramSum returns the sum of observations recorded for the named
// histogram and label combination.
func (a *Adapter) HistogramSum(name string, labels map[string]string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	series, ok := a.histograms[name]
	if !ok {
		return 0
	}
	entry, ok := series[labelKey(labels)]
	if !ok {
		return 0
	}
	return entry.sum
}

// Exists reports whether a metric with the given name has been
// registered via [Adapter.Register].
func (a *Adapter) Exists(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.registered[name]
	return ok
}

// Names returns the names of all registered metrics, sorted.
func (a *Adapter) Names() []string {
	a.mu.RLock()
	out := make([]string, 0, len(a.registered))
	for name := range a.registered {
		out = append(out, name)
	}
	a.mu.RUnlock()
	slices.Sort(out)
	return out
}

// labelKey serializes a label set into a deterministic string for
// indexing. Order is fixed by sorting keys, so equivalent label maps
// always collapse onto the same bucket regardless of iteration order.
func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

// Ensure Adapter implements [adapters.Adapter].
var _ adapters.Adapter = (*Adapter)(nil)
