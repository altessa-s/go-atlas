// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// Adapter implements the adapters.Adapter interface for Prometheus.
// It also implements adapters.HTTPHandler, adapters.Binder, and adapters.GaugeAdder.
//
// Metrics are registered once and read on every observation, so sync.Map
// (optimized for read-heavy workloads) replaces the former RWMutex+map.
type Adapter struct {
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer

	descsMu  sync.Mutex
	descs    map[string]*adapters.Desc
	counters sync.Map // string -> *prometheus.CounterVec
	gauges   sync.Map // string -> *prometheus.GaugeVec
	histos   sync.Map // string -> *prometheus.HistogramVec

	droppedObservations atomic.Uint64
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
		descs:      make(map[string]*adapters.Desc),
	}
}

// Name implements adapters.Adapter.
func (a *Adapter) Name() string {
	return "prometheus"
}

// Register implements adapters.Adapter.
func (a *Adapter) Register(desc *adapters.Desc) error {
	if desc == nil {
		return fmt.Errorf("prometheus: nil descriptor")
	}

	norm := desc.Clone()
	if norm.Type == adapters.TypeHistogram && len(norm.Buckets) == 0 {
		norm.Buckets = slices.Clone(metrics.DefaultDurationBuckets)
	}

	a.descsMu.Lock()
	defer a.descsMu.Unlock()

	if existing, ok := a.descs[norm.Name]; ok {
		if !existing.Equal(norm) {
			return fmt.Errorf("prometheus: metric %q: %w", norm.Name, adapters.ErrDescriptorConflict)
		}
		return nil
	}

	var err error
	switch norm.Type {
	case adapters.TypeCounter:
		vec := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: norm.Name,
			Help: norm.Help,
		}, norm.LabelNames)
		err = registerVec(a.registerer, &a.counters, norm.Name, vec)

	case adapters.TypeGauge:
		vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: norm.Name,
			Help: norm.Help,
		}, norm.LabelNames)
		err = registerVec(a.registerer, &a.gauges, norm.Name, vec)

	case adapters.TypeHistogram:
		vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    norm.Name,
			Help:    norm.Help,
			Buckets: norm.Buckets,
		}, norm.LabelNames)
		err = registerVec(a.registerer, &a.histos, norm.Name, vec)

	default:
		return fmt.Errorf("prometheus: unsupported metric type %v", norm.Type)
	}

	if err != nil {
		return err
	}

	a.descs[norm.Name] = norm
	return nil
}

// registerVec registers vec and stores it in m under name. When an identical
// collector is already registered (e.g. by another adapter sharing the
// registry), the existing vector is attached instead so observations reach
// it; an existing collector of a different type is an error.
func registerVec[V prometheus.Collector](reg prometheus.Registerer, m *sync.Map, name string, vec V) error {
	if err := reg.Register(vec); err != nil {
		var are prometheus.AlreadyRegisteredError
		if !errors.As(err, &are) {
			return err
		}
		existing, ok := are.ExistingCollector.(V)
		if !ok {
			return fmt.Errorf("prometheus: metric %q already registered as %T: %w: %w",
				name, are.ExistingCollector, adapters.ErrDescriptorConflict, err)
		}
		vec = existing
	}
	m.Store(name, vec)
	return nil
}

// RecordCounter implements adapters.Adapter.
func (a *Adapter) RecordCounter(name string, labels map[string]string, delta float64) {
	raw, ok := a.counters.Load(name)
	if !ok {
		return
	}
	vec, ok := raw.(*prometheus.CounterVec)
	if !ok {
		return
	}
	c, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		a.droppedObservations.Add(1)
		return
	}
	c.Add(delta)
}

// RecordGauge implements adapters.Adapter.
func (a *Adapter) RecordGauge(name string, labels map[string]string, value float64) {
	raw, ok := a.gauges.Load(name)
	if !ok {
		return
	}
	vec, ok := raw.(*prometheus.GaugeVec)
	if !ok {
		return
	}
	g, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		a.droppedObservations.Add(1)
		return
	}
	g.Set(value)
}

// AddGauge implements adapters.GaugeAdder.
func (a *Adapter) AddGauge(name string, labels map[string]string, delta float64) {
	raw, ok := a.gauges.Load(name)
	if !ok {
		return
	}
	vec, ok := raw.(*prometheus.GaugeVec)
	if !ok {
		return
	}
	g, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		a.droppedObservations.Add(1)
		return
	}
	g.Add(delta)
}

// RecordHistogram implements adapters.Adapter.
func (a *Adapter) RecordHistogram(name string, labels map[string]string, value float64) {
	raw, ok := a.histos.Load(name)
	if !ok {
		return
	}
	vec, ok := raw.(*prometheus.HistogramVec)
	if !ok {
		return
	}
	h, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		a.droppedObservations.Add(1)
		return
	}
	h.Observe(value)
}

// BindCounter implements adapters.Binder: the Prometheus child is resolved
// once via GetMetricWith so Add on the returned handle is a direct atomic
// update with no per-observation vector lookup or label hashing.
func (a *Adapter) BindCounter(name string, labels map[string]string) (adapters.BoundCounter, bool) {
	raw, ok := a.counters.Load(name)
	if !ok {
		return nil, false
	}
	vec, ok := raw.(*prometheus.CounterVec)
	if !ok {
		return nil, false
	}
	c, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		return nil, false
	}
	return c, true
}

// BindGauge implements adapters.Binder.
func (a *Adapter) BindGauge(name string, labels map[string]string) (adapters.BoundGauge, bool) {
	raw, ok := a.gauges.Load(name)
	if !ok {
		return nil, false
	}
	vec, ok := raw.(*prometheus.GaugeVec)
	if !ok {
		return nil, false
	}
	g, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		return nil, false
	}
	return g, true
}

// BindHistogram implements adapters.Binder.
func (a *Adapter) BindHistogram(name string, labels map[string]string) (adapters.BoundHistogram, bool) {
	raw, ok := a.histos.Load(name)
	if !ok {
		return nil, false
	}
	vec, ok := raw.(*prometheus.HistogramVec)
	if !ok {
		return nil, false
	}
	o, err := vec.GetMetricWith(normalizeLabels(labels))
	if err != nil {
		return nil, false
	}
	return o, true
}

// DroppedObservations returns the number of observations dropped due to label
// cardinality or name mismatches on the unbound recording path.
func (a *Adapter) DroppedObservations() uint64 {
	return a.droppedObservations.Load()
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

// Handler implements adapters.HTTPHandler.
// Returns an http.Handler that serves metrics in Prometheus format.
func (a *Adapter) Handler() http.Handler {
	return promhttp.HandlerFor(a.gatherer, promhttp.HandlerOpts{DisableCompression: true})
}

// DefaultHandler returns an http.Handler that serves the process-wide
// default Prometheus gatherer without compression, the exposition Prometheus
// scrapers expect. Pass it to the HTTP server factory's UseMetricsHandler when
// the adapter registers in the default registry.
func DefaultHandler() http.Handler {
	return promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true})
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
	_ adapters.GaugeAdder  = (*Adapter)(nil)
)
