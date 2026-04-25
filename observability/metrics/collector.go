// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/internal/shared"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// collector is the default implementation of Collector.
type collector struct {
	serviceName string
	adapter     adapters.Adapter
	metrics     sync.Map // map[string]any (Counter, Gauge, etc.)
	shutdown    atomic.Bool
}

// New creates a new [Collector] with the given options.
// Returns [Noop] if no adapter is configured.
func New(opts ...Option) Collector {
	cfg := newOptions(opts...)

	// If no adapter configured, return noop collector
	if cfg.adapter == nil {
		return Noop()
	}

	return &collector{
		serviceName: cfg.serviceName,
		adapter:     cfg.adapter,
	}
}

// Counter implements Collector.
func (r *collector) Counter(opts MetricOpts) Counter {
	return r.getOrCreateCounter("", opts)
}

// Gauge implements Collector.
func (r *collector) Gauge(opts MetricOpts) Gauge {
	return r.getOrCreateGauge("", opts)
}

// Histogram implements Collector.
func (r *collector) Histogram(opts HistogramOpts) Histogram {
	return r.getOrCreateHistogram("", opts)
}

// Timer implements Collector.
func (r *collector) Timer(opts HistogramOpts) Timer {
	return r.getOrCreateTimer("", opts)
}

// MustCounter implements Collector.
func (r *collector) MustCounter(opts MetricOpts) Counter {
	return must(opts, func() Counter { return r.Counter(opts) })
}

// MustGauge implements Collector.
func (r *collector) MustGauge(opts MetricOpts) Gauge {
	return must(opts, func() Gauge { return r.Gauge(opts) })
}

// MustHistogram implements Collector.
func (r *collector) MustHistogram(opts HistogramOpts) Histogram {
	return must(opts, func() Histogram { return r.Histogram(opts) })
}

// MustTimer implements Collector.
func (r *collector) MustTimer(opts HistogramOpts) Timer {
	return must(opts, func() Timer { return r.Timer(opts) })
}

// WithSubsystem implements Collector.
func (r *collector) WithSubsystem(subsystem string) Collector {
	return &scopedCollector{
		parent:    r,
		subsystem: subsystem,
	}
}

// Shutdown implements Collector.
func (r *collector) Shutdown(ctx context.Context) error {
	if r.shutdown.Load() {
		return nil // Already shutdown
	}

	// Flush first (before setting shutdown flag)
	if err := r.adapter.Flush(); err != nil {
		return err
	}

	// Mark as shutdown
	if !r.shutdown.CompareAndSwap(false, true) {
		return nil // Already shutdown (race condition)
	}

	// Close adapter
	return r.adapter.Close()
}

// ForceFlush implements Collector.
func (r *collector) ForceFlush(ctx context.Context) error {
	if r.shutdown.Load() {
		return ErrCollectorShutdown
	}
	return r.adapter.Flush()
}

// validator is an interface for types that can be validated.
type validator interface {
	Validate() error
}

// must validates opts and panics if validation fails, otherwise calls create.
func must[T any](opts validator, create func() T) T {
	if err := opts.Validate(); err != nil {
		panic(err)
	}
	return create()
}

// getOrCreateCounter retrieves or creates a counter.
func (r *collector) getOrCreateCounter(subsystem string, opts MetricOpts) Counter {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)

	return shared.GetOrCreateWithCallback(&r.metrics, fullName,
		func() *counter {
			return newCounter(fullName, opts.LabelNames, r.adapter)
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeCounter, opts.LabelNames, nil)
		},
	)
}

// getOrCreateGauge retrieves or creates a gauge.
func (r *collector) getOrCreateGauge(subsystem string, opts MetricOpts) Gauge {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)

	return shared.GetOrCreateWithCallback(&r.metrics, fullName,
		func() *gauge {
			return newGauge(fullName, opts.LabelNames, r.adapter)
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeGauge, opts.LabelNames, nil)
		},
	)
}

// getOrCreateHistogram retrieves or creates a histogram.
func (r *collector) getOrCreateHistogram(subsystem string, opts HistogramOpts) Histogram {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	buckets := bucketsOrDefault(opts.Buckets)

	return shared.GetOrCreateWithCallback(&r.metrics, fullName,
		func() *histogram {
			return newHistogram(fullName, opts.LabelNames, buckets, r.adapter)
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeHistogram, opts.LabelNames, buckets)
		},
	)
}

// getOrCreateTimer retrieves or creates a timer.
func (r *collector) getOrCreateTimer(subsystem string, opts HistogramOpts) Timer {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	buckets := bucketsOrDefault(opts.Buckets)

	return shared.GetOrCreateWithCallback(&r.metrics, fullName,
		func() *timer {
			return newTimer(fullName, opts.LabelNames, buckets, r.adapter)
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeHistogram, opts.LabelNames, buckets)
		},
	)
}

// register registers a metric descriptor with the adapter.
// Registration errors are intentionally ignored as adapters may have different
// registration requirements (e.g., Prometheus doesn't require pre-registration).
func (r *collector) register(name, help string, typ adapters.MetricType, labelNames []string, buckets []float64) {
	_ = r.adapter.Register(&adapters.Desc{ //nolint:errcheck
		Name:       name,
		Help:       help,
		Type:       typ,
		LabelNames: labelNames,
		Buckets:    buckets,
	})
}

// bucketsOrDefault returns buckets or DefaultDurationBuckets if empty.
func bucketsOrDefault(buckets []float64) []float64 {
	if len(buckets) == 0 {
		return DefaultDurationBuckets
	}
	return buckets
}
