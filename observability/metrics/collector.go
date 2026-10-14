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
	metrics     sync.Map   // map[string]any (Counter, Gauge, etc.)
	createMu    sync.Mutex // serializes metric creation + registration before publication
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

	// Unlabeled metrics bind their child after registration and before the
	// handle is published, so observations skip the per-call lookups.
	var created *counter
	return shared.GetOrCreateWithCallback(&r.metrics, &r.createMu, fullName,
		func() *counter {
			created = newCounter(fullName, r.adapter)
			return created
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeCounter, opts.LabelNames, nil)
			if len(opts.LabelNames) == 0 {
				created.bindUnlabeled()
			}
		},
	)
}

// getOrCreateGauge retrieves or creates a gauge.
func (r *collector) getOrCreateGauge(subsystem string, opts MetricOpts) Gauge {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)

	// Unlabeled metrics bind their child after registration and before the
	// handle is published, so observations skip the per-call lookups.
	var created *gauge
	return shared.GetOrCreateWithCallback(&r.metrics, &r.createMu, fullName,
		func() *gauge {
			created = newGauge(fullName, r.adapter)
			return created
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeGauge, opts.LabelNames, nil)
			if len(opts.LabelNames) == 0 {
				created.bindUnlabeled()
			}
		},
	)
}

// getOrCreateHistogram retrieves or creates a histogram.
func (r *collector) getOrCreateHistogram(subsystem string, opts HistogramOpts) Histogram {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	buckets := bucketsOrDefault(opts.Buckets)

	// Unlabeled metrics bind their child after registration and before the
	// handle is published, so observations skip the per-call lookups.
	var created *histogram
	return shared.GetOrCreateWithCallback(&r.metrics, &r.createMu, fullName,
		func() *histogram {
			created = newHistogram(fullName, r.adapter)
			return created
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeHistogram, opts.LabelNames, buckets)
			if len(opts.LabelNames) == 0 {
				created.bindUnlabeled()
			}
		},
	)
}

// getOrCreateTimer retrieves or creates a timer.
func (r *collector) getOrCreateTimer(subsystem string, opts HistogramOpts) Timer {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	buckets := bucketsOrDefault(opts.Buckets)

	// Unlabeled metrics bind their child after registration and before the
	// handle is published, so observations skip the per-call lookups.
	var created *timer
	return shared.GetOrCreateWithCallback(&r.metrics, &r.createMu, fullName,
		func() *timer {
			created = newTimer(fullName, r.adapter)
			return created
		},
		func() {
			r.register(fullName, opts.Help, adapters.TypeHistogram, opts.LabelNames, buckets)
			if len(opts.LabelNames) == 0 {
				created.bindUnlabeled()
			}
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
