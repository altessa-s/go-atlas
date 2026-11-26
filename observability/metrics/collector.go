// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/internal/shared"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// activeLifecycle is a permanently false lifecycle flag used as a non-nil
// fallback when internal constructors are called without a collector flag.
var activeLifecycle atomic.Bool

type instrumentKind uint8

const (
	kindCounter instrumentKind = iota + 1
	kindGauge
	kindHistogram
	kindTimer
)

func (k instrumentKind) String() string {
	switch k {
	case kindCounter:
		return "counter"
	case kindGauge:
		return "gauge"
	case kindHistogram:
		return "histogram"
	case kindTimer:
		return "timer"
	default:
		return "unknown"
	}
}

func (k instrumentKind) adapterType() adapters.MetricType {
	switch k {
	case kindCounter:
		return adapters.TypeCounter
	case kindGauge:
		return adapters.TypeGauge
	case kindHistogram, kindTimer:
		return adapters.TypeHistogram
	default:
		return adapters.TypeCounter
	}
}

// descriptor captures the canonical identity of a published metric name.
type descriptor struct {
	kind       instrumentKind
	labelNames []string
	help       string
	buckets    []float64
}

func (d descriptor) equal(other descriptor) bool {
	return d.kind == other.kind &&
		d.help == other.help &&
		slices.Equal(d.labelNames, other.labelNames) &&
		slices.Equal(d.buckets, other.buckets)
}

type metricEntry struct {
	desc   descriptor
	handle any
}

func entryKindString(e *metricEntry) string {
	if e == nil {
		return "unknown"
	}
	return e.desc.kind.String()
}

// collector is the default implementation of Collector.
type collector struct {
	serviceName  string
	adapter      adapters.Adapter
	logger       *slog.Logger
	entries      sync.Map   // fullName (string) -> *metricEntry
	loggedErrors sync.Map   // keys.Compose(fullName, kind) -> struct{}
	createMu     sync.Mutex // serializes metric creation + registration + shutdown transition
	shutdown     atomic.Bool
	shutdownOnce sync.Once
	shutdownErr  error
}

// New creates a new [Collector] with the given options.
// Returns [Noop] if no adapter is configured.
func New(opts ...Option) Collector {
	cfg := newOptions(opts...)

	// If no adapter configured, return noop collector
	if nilcheck.IsNil(cfg.adapter) {
		return Noop()
	}

	logger := cfg.logger
	if logger == nil {
		logger = slog.Default()
	}

	return &collector{
		serviceName: cfg.serviceName,
		adapter:     cfg.adapter,
		logger:      logger,
	}
}

// Counter implements Collector.
func (r *collector) Counter(opts MetricOpts) Counter {
	c, err := r.tryGetOrCreateCounter("", opts)
	if err != nil {
		r.recordError(shared.BuildMetricName(r.serviceName, "", opts.Name), kindCounter, err)
		return noopCounter{}
	}
	return c
}

// Gauge implements Collector.
func (r *collector) Gauge(opts MetricOpts) Gauge {
	g, err := r.tryGetOrCreateGauge("", opts)
	if err != nil {
		r.recordError(shared.BuildMetricName(r.serviceName, "", opts.Name), kindGauge, err)
		return noopGauge{}
	}
	return g
}

// Histogram implements Collector.
func (r *collector) Histogram(opts HistogramOpts) Histogram {
	h, err := r.tryGetOrCreateHistogram("", opts)
	if err != nil {
		r.recordError(shared.BuildMetricName(r.serviceName, "", opts.Name), kindHistogram, err)
		return noopHistogram{}
	}
	return h
}

// Timer implements Collector.
func (r *collector) Timer(opts HistogramOpts) Timer {
	tm, err := r.tryGetOrCreateTimer("", opts)
	if err != nil {
		r.recordError(shared.BuildMetricName(r.serviceName, "", opts.Name), kindTimer, err)
		return noopTimer{}
	}
	return tm
}

// RegisterCounter implements Collector.
func (r *collector) RegisterCounter(opts MetricOpts) (Counter, error) {
	return r.tryGetOrCreateCounter("", opts)
}

// RegisterGauge implements Collector.
func (r *collector) RegisterGauge(opts MetricOpts) (Gauge, error) {
	return r.tryGetOrCreateGauge("", opts)
}

// RegisterHistogram implements Collector.
func (r *collector) RegisterHistogram(opts HistogramOpts) (Histogram, error) {
	return r.tryGetOrCreateHistogram("", opts)
}

// RegisterTimer implements Collector.
func (r *collector) RegisterTimer(opts HistogramOpts) (Timer, error) {
	return r.tryGetOrCreateTimer("", opts)
}

// WithSubsystem implements Collector.
func (r *collector) WithSubsystem(subsystem string) Meter {
	return &scopedMeter{
		parent:    r,
		subsystem: subsystem,
	}
}

// Shutdown implements Collector.
func (r *collector) Shutdown(_ context.Context) error {
	r.shutdownOnce.Do(func() {
		flushErr := r.flush()

		r.createMu.Lock()
		r.shutdown.Store(true)
		r.createMu.Unlock()

		var closeErr error
		if closer, ok := r.adapter.(adapters.Closer); ok {
			closeErr = closer.Close()
		}
		r.shutdownErr = errors.Join(flushErr, closeErr)
	})
	return r.shutdownErr
}

// ForceFlush implements Collector.
func (r *collector) ForceFlush(_ context.Context) error {
	if r.shutdown.Load() {
		return ErrCollectorShutdown
	}
	return r.flush()
}

func (r *collector) flush() error {
	if flusher, ok := r.adapter.(adapters.Flusher); ok {
		return flusher.Flush()
	}
	return nil
}

func (r *collector) tryGetOrCreateCounter(subsystem string, opts MetricOpts) (Counter, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindCounter,
		labelNames: NormalizeLabelNames(opts.LabelNames),
		help:       opts.Help,
	}
	return registerOrGet(r, fullName, desc, opts.Validate(), func(name string) (Counter, func()) {
		c := newCounter(name, r.adapter, &r.shutdown)
		return c, c.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateGauge(subsystem string, opts MetricOpts) (Gauge, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindGauge,
		labelNames: NormalizeLabelNames(opts.LabelNames),
		help:       opts.Help,
	}
	return registerOrGet(r, fullName, desc, opts.Validate(), func(name string) (Gauge, func()) {
		g := newGauge(name, r.adapter, &r.shutdown)
		return g, g.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateHistogram(subsystem string, opts HistogramOpts) (Histogram, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindHistogram,
		labelNames: NormalizeLabelNames(opts.LabelNames),
		help:       opts.Help,
		buckets:    slices.Clone(bucketsOrDefault(opts.Buckets)),
	}
	return registerOrGet(r, fullName, desc, opts.Validate(), func(name string) (Histogram, func()) {
		h := newHistogram(name, r.adapter, &r.shutdown)
		return h, h.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateTimer(subsystem string, opts HistogramOpts) (Timer, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindTimer,
		labelNames: NormalizeLabelNames(opts.LabelNames),
		help:       opts.Help,
		buckets:    slices.Clone(bucketsOrDefault(opts.Buckets)),
	}
	return registerOrGet(r, fullName, desc, opts.Validate(), func(name string) (Timer, func()) {
		tm := newTimer(name, r.adapter, &r.shutdown)
		return tm, tm.bindUnlabeled
	})
}

func registerOrGet[T any](
	r *collector,
	fullName string,
	desc descriptor,
	validateErr error,
	create func(fullName string) (T, func()),
) (T, error) {
	var zero T
	if r.shutdown.Load() {
		return zero, ErrCollectorShutdown
	}
	if validateErr != nil {
		return zero, validateErr
	}

	// Fast path: already published.
	if raw, ok := r.entries.Load(fullName); ok {
		entry, ok := raw.(*metricEntry)
		if !ok || !entry.desc.equal(desc) {
			return zero, fmt.Errorf("metric %q (existing %s, requested %s): %w",
				fullName, entryKindString(entry), desc.kind, ErrDescriptorConflict)
		}
		if h, ok := entry.handle.(T); ok {
			return h, nil
		}
		return zero, fmt.Errorf("metric %q: %w", fullName, ErrDescriptorConflict)
	}

	r.createMu.Lock()
	defer r.createMu.Unlock()

	if r.shutdown.Load() {
		return zero, ErrCollectorShutdown
	}

	// Re-check: another caller may have published it while we waited.
	if raw, ok := r.entries.Load(fullName); ok {
		entry, ok := raw.(*metricEntry)
		if !ok || !entry.desc.equal(desc) {
			return zero, fmt.Errorf("metric %q (existing %s, requested %s): %w",
				fullName, entryKindString(entry), desc.kind, ErrDescriptorConflict)
		}
		if h, ok := entry.handle.(T); ok {
			return h, nil
		}
		return zero, fmt.Errorf("metric %q: %w", fullName, ErrDescriptorConflict)
	}

	if err := r.adapter.Register(&adapters.Desc{
		Name:       fullName,
		Help:       desc.help,
		Type:       desc.kind.adapterType(),
		LabelNames: desc.labelNames,
		Buckets:    desc.buckets,
	}); err != nil {
		return zero, coreerrs.WrapOperationWithContext(err, "register metric", r.adapter.Name())
	}

	// Unlabeled metrics bind their child after registration and before the
	// handle is published, so observations skip per-call lookups.
	h, bind := create(fullName)
	if len(desc.labelNames) == 0 && bind != nil {
		bind()
	}
	r.entries.Store(fullName, &metricEntry{
		desc:   desc,
		handle: h,
	})
	return h, nil
}

func (r *collector) recordError(fullName string, kind instrumentKind, err error) {
	if errors.Is(err, ErrCollectorShutdown) {
		return
	}
	key := keys.Compose(fullName, kind.String())
	if _, loaded := r.loggedErrors.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	reason := "register_failed"
	switch {
	case errors.Is(err, ErrDescriptorConflict):
		reason = "descriptor_conflict"
	case errors.Is(err, ErrEmptyMetricName), errors.Is(err, ErrEmptyLabelName):
		reason = "invalid_options"
	}
	r.logger.Error("metric registration failed",
		slog.String("metric", fullName),
		slog.String("kind", kind.String()),
		slog.String("adapter", r.adapter.Name()),
		slog.String("reason", reason),
	)
}

// bucketsOrDefault returns buckets or DefaultDurationBuckets if empty.
func bucketsOrDefault(buckets []float64) []float64 {
	if len(buckets) == 0 {
		return DefaultDurationBuckets
	}
	return buckets
}
