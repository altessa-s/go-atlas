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
//
// Metrics are registered once and read on every observation, so sync.Map
// (optimized for read-heavy workloads) replaces the former RWMutex+map.
type Adapter struct {
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer

	counters sync.Map // string → *prometheus.CounterVec
	gauges   sync.Map // string → *prometheus.GaugeVec
	histos   sync.Map // string → *prometheus.HistogramVec
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
	}
}

// Name implements adapters.Adapter.
func (a *Adapter) Name() string {
	return "prometheus"
}

// Register implements adapters.Adapter.
func (a *Adapter) Register(desc *adapters.Desc) error {
	switch desc.Type {
	case adapters.TypeCounter:
		if _, ok := a.counters.Load(desc.Name); ok {
			return nil
		}
		vec := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: desc.Name,
			Help: desc.Help,
		}, desc.LabelNames)
		if err := a.registerer.Register(vec); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
				return nil
			}
			return err
		}
		a.counters.Store(desc.Name, vec)

	case adapters.TypeGauge:
		if _, ok := a.gauges.Load(desc.Name); ok {
			return nil
		}
		vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: desc.Name,
			Help: desc.Help,
		}, desc.LabelNames)
		if err := a.registerer.Register(vec); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
				return nil
			}
			return err
		}
		a.gauges.Store(desc.Name, vec)

	case adapters.TypeHistogram:
		if _, ok := a.histos.Load(desc.Name); ok {
			return nil
		}
		buckets := desc.Buckets
		if len(buckets) == 0 {
			buckets = metrics.DefaultDurationBuckets
		}
		vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    desc.Name,
			Help:    desc.Help,
			Buckets: buckets,
		}, desc.LabelNames)
		if err := a.registerer.Register(vec); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
				return nil
			}
			return err
		}
		a.histos.Store(desc.Name, vec)
	}

	return nil
}

// RecordCounter implements adapters.Adapter.
func (a *Adapter) RecordCounter(name string, labels map[string]string, delta float64) {
	if vec, ok := a.counters.Load(name); ok {
		vec.(*prometheus.CounterVec).With(normalizeLabels(labels)).Add(delta) //nolint:errcheck // type guaranteed by Store
	}
}

// RecordGauge implements adapters.Adapter.
func (a *Adapter) RecordGauge(name string, labels map[string]string, value float64) {
	if vec, ok := a.gauges.Load(name); ok {
		vec.(*prometheus.GaugeVec).With(normalizeLabels(labels)).Set(value) //nolint:errcheck // type guaranteed by Store
	}
}

// RecordHistogram implements adapters.Adapter.
func (a *Adapter) RecordHistogram(name string, labels map[string]string, value float64) {
	if vec, ok := a.histos.Load(name); ok {
		vec.(*prometheus.HistogramVec).With(normalizeLabels(labels)).Observe(value) //nolint:errcheck // type guaranteed by Store
	}
}

// BindCounter implements adapters.Binder: the Prometheus child is resolved
// once via GetMetricWith so Add on the returned handle is a direct atomic
// update with no per-observation vector lookup or label hashing.
func (a *Adapter) BindCounter(name string, labels map[string]string) (adapters.BoundCounter, bool) {
	vec, ok := a.counters.Load(name)
	if !ok {
		return nil, false
	}
	c, err := vec.(*prometheus.CounterVec).GetMetricWith(normalizeLabels(labels)) //nolint:errcheck // type guaranteed by Store
	if err != nil {
		return nil, false
	}
	return c, true
}

// BindGauge implements adapters.Binder.
func (a *Adapter) BindGauge(name string, labels map[string]string) (adapters.BoundGauge, bool) {
	vec, ok := a.gauges.Load(name)
	if !ok {
		return nil, false
	}
	g, err := vec.(*prometheus.GaugeVec).GetMetricWith(normalizeLabels(labels)) //nolint:errcheck // type guaranteed by Store
	if err != nil {
		return nil, false
	}
	return g, true
}

// BindHistogram implements adapters.Binder.
func (a *Adapter) BindHistogram(name string, labels map[string]string) (adapters.BoundHistogram, bool) {
	vec, ok := a.histos.Load(name)
	if !ok {
		return nil, false
	}
	o, err := vec.(*prometheus.HistogramVec).GetMetricWith(normalizeLabels(labels)) //nolint:errcheck // type guaranteed by Store
	if err != nil {
		return nil, false
	}
	return o, true
}

// emptyLabels is a pre-allocated empty label set, avoiding a heap allocation
// on every unlabeled metric observation.
var emptyLabels = prometheus.Labels{}

// normalizeLabels returns labels or empty Labels if nil.
func normalizeLabels(labels map[string]string) prometheus.Labels {
	if labels == nil {
		return emptyLabels
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
	return promhttp.HandlerFor(a.gatherer, promhttp.HandlerOpts{DisableCompression: true})
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
	_ adapters.Binder      = (*Adapter)(nil)
)
