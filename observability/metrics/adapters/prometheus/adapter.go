// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// Adapter implements the adapters.Adapter interface for Prometheus.
// It also implements adapters.HTTPHandler for exposing metrics via HTTP.
type Adapter struct {
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer

	mu       sync.RWMutex
	counters map[string]*prometheus.CounterVec
	gauges   map[string]*prometheus.GaugeVec
	histos   map[string]*prometheus.HistogramVec
}

// New creates a new Prometheus adapter with the given options.
func New(opts ...Option) *Adapter {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	registerer := cfg.registerer
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	gatherer := cfg.gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	return &Adapter{
		registerer: registerer,
		gatherer:   gatherer,
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histos:     make(map[string]*prometheus.HistogramVec),
	}
}

// Name implements adapters.Adapter.
func (a *Adapter) Name() string {
	return "prometheus"
}

// Register implements adapters.Adapter.
func (a *Adapter) Register(desc *adapters.Desc) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch desc.Type {
	case adapters.TypeCounter:
		return registerMetric(a.registerer, a.counters, desc.Name, func() *prometheus.CounterVec {
			return prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: desc.Name,
				Help: desc.Help,
			}, desc.LabelNames)
		})

	case adapters.TypeGauge:
		return registerMetric(a.registerer, a.gauges, desc.Name, func() *prometheus.GaugeVec {
			return prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: desc.Name,
				Help: desc.Help,
			}, desc.LabelNames)
		})

	case adapters.TypeHistogram:
		buckets := desc.Buckets
		if len(buckets) == 0 {
			buckets = metrics.DefaultDurationBuckets
		}
		return registerMetric(a.registerer, a.histos, desc.Name, func() *prometheus.HistogramVec {
			return prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    desc.Name,
				Help:    desc.Help,
				Buckets: buckets,
			}, desc.LabelNames)
		})
	}

	return nil
}

// registerMetric is a generic helper for registering a metric with Prometheus.
func registerMetric[T prometheus.Collector](
	registerer prometheus.Registerer,
	m map[string]T,
	name string,
	create func() T,
) error {
	if _, exists := m[name]; exists {
		return nil
	}

	vec := create()
	if err := registerer.Register(vec); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return nil
		}
		return err
	}
	m[name] = vec
	return nil
}

// RecordCounter implements adapters.Adapter.
func (a *Adapter) RecordCounter(name string, labels map[string]string, delta float64) {
	withMetric(a, name, a.counters, func(vec *prometheus.CounterVec) {
		vec.With(normalizeLabels(labels)).Add(delta)
	})
}

// RecordGauge implements adapters.Adapter.
func (a *Adapter) RecordGauge(name string, labels map[string]string, value float64) {
	withMetric(a, name, a.gauges, func(vec *prometheus.GaugeVec) {
		vec.With(normalizeLabels(labels)).Set(value)
	})
}

// RecordHistogram implements adapters.Adapter.
func (a *Adapter) RecordHistogram(name string, labels map[string]string, value float64) {
	withMetric(a, name, a.histos, func(vec *prometheus.HistogramVec) {
		vec.With(normalizeLabels(labels)).Observe(value)
	})
}

// withMetric is a generic helper that safely retrieves a metric vec and calls fn if found.
func withMetric[T any](a *Adapter, name string, m map[string]T, fn func(T)) {
	a.mu.RLock()
	vec, ok := m[name]
	a.mu.RUnlock()
	if ok {
		fn(vec)
	}
}

// normalizeLabels returns labels or empty Labels if nil.
func normalizeLabels(labels map[string]string) prometheus.Labels {
	if labels == nil {
		return prometheus.Labels{}
	}
	return labels
}

// Flush implements adapters.Adapter.
// For Prometheus, this is a no-op as metrics are scraped by Prometheus.
func (a *Adapter) Flush() error {
	return nil
}

// Close implements adapters.Adapter.
func (a *Adapter) Close() error {
	return nil
}

// Handler implements adapters.HTTPHandler.
// Returns an http.Handler that serves metrics in Prometheus format.
func (a *Adapter) Handler() http.Handler {
	return promhttp.HandlerFor(a.gatherer, promhttp.HandlerOpts{})
}

// Registerer returns the underlying Prometheus registerer.
func (a *Adapter) Registerer() prometheus.Registerer {
	return a.registerer
}

// Gatherer returns the underlying Prometheus gatherer.
func (a *Adapter) Gatherer() prometheus.Gatherer {
	return a.gatherer
}

// Ensure Adapter implements the required interfaces.
var (
	_ adapters.Adapter     = (*Adapter)(nil)
	_ adapters.HTTPHandler = (*Adapter)(nil)
)
