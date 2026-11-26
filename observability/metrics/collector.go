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
	case kindHistogram:
		return adapters.TypeHistogram
	case kindTimer:
		return adapters.TypeTimer
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

func (d descriptor) matches(other descriptor) bool {
	return d.kind == other.kind &&
		d.help == other.help &&
		equalLabelNames(d.labelNames, other.labelNames) &&
		slices.Equal(d.buckets, other.buckets)
}

// equalLabelNames compares the registered sorted names with the caller's
// names without sorting or allocating on a compatible redeclaration.
func equalLabelNames(sorted, requested []string) bool {
	if len(sorted) != len(requested) {
		return false
	}
	if slices.Equal(sorted, requested) {
		return true
	}
	for first := 0; first < len(sorted); {
		name := sorted[first]
		last := first + 1
		for last < len(sorted) && sorted[last] == name {
			last++
		}
		var count int
		for _, label := range requested {
			if label == name {
				count++
			}
		}
		if count != last-first {
			return false
		}
		first = last
	}
	return true
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
	recording    adapters.Adapter
	admission    *admission
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

	r := &collector{
		serviceName: cfg.serviceName,
		adapter:     cfg.adapter,
		recording:   cfg.adapter,
		logger:      logger,
	}
	_, flushes := cfg.adapter.(adapters.Flusher)
	_, closes := cfg.adapter.(adapters.Closer)
	if flushes || closes {
		r.admission = &admission{closed: &r.shutdown}
		guarded := &guardedAdapter{Adapter: cfg.adapter, gate: r.admission}
		r.recording = guarded
		if adder, ok := cfg.adapter.(adapters.GaugeAdder); ok {
			r.recording = &guardedAdapterAdder{guardedAdapter: guarded, adder: adder}
		}
	}
	return r
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
		r.createMu.Lock()
		r.shutdown.Store(true)
		r.createMu.Unlock()

		if r.admission != nil {
			r.admission.mu.Lock()
			r.admission.mu.Unlock() //nolint:staticcheck // The lock acquisition waits for accepted backend calls.
		}
		flushErr := r.flush()

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
	if r.admission != nil {
		if !r.admission.enter() {
			return ErrCollectorShutdown
		}
		defer r.admission.mu.RUnlock()
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
		labelNames: opts.LabelNames,
		help:       opts.Help,
	}
	return registerOrGet(r, fullName, opts.Name, desc, opts.Validate, func(name string) (Counter, func()) {
		c := newCounter(name, r.recording, &r.shutdown)
		return c, c.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateGauge(subsystem string, opts MetricOpts) (Gauge, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindGauge,
		labelNames: opts.LabelNames,
		help:       opts.Help,
	}
	return registerOrGet(r, fullName, opts.Name, desc, opts.Validate, func(name string) (Gauge, func()) {
		g := newGauge(name, r.recording, &r.shutdown)
		return g, g.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateHistogram(subsystem string, opts HistogramOpts) (Histogram, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindHistogram,
		labelNames: opts.LabelNames,
		help:       opts.Help,
		buckets:    bucketsOrDefault(opts.Buckets),
	}
	return registerOrGet(r, fullName, opts.Name, desc, opts.Validate, func(name string) (Histogram, func()) {
		h := newHistogram(name, r.recording, &r.shutdown)
		return h, h.bindUnlabeled
	})
}

func (r *collector) tryGetOrCreateTimer(subsystem string, opts HistogramOpts) (Timer, error) {
	fullName := shared.BuildMetricName(r.serviceName, subsystem, opts.Name)
	desc := descriptor{
		kind:       kindTimer,
		labelNames: opts.LabelNames,
		help:       opts.Help,
		buckets:    bucketsOrDefault(opts.Buckets),
	}
	return registerOrGet(r, fullName, opts.Name, desc, opts.Validate, func(name string) (Timer, func()) {
		tm := newTimer(name, r.recording, &r.shutdown)
		return tm, tm.bindUnlabeled
	})
}

func registerOrGet[T any](
	r *collector,
	fullName, name string,
	desc descriptor,
	validate func() error,
	create func(fullName string) (T, func()),
) (T, error) {
	var zero T
	if r.shutdown.Load() {
		return zero, ErrCollectorShutdown
	}
	if name == "" {
		return zero, ErrEmptyMetricName
	}

	// A matching descriptor has already been validated and registered.
	if raw, ok := r.entries.Load(fullName); ok {
		if handle, ok := matchEntry[T](raw, desc); ok {
			return handle, nil
		}
		if err := validate(); err != nil {
			return zero, err
		}
		return zero, descriptorConflict(fullName, raw, desc.kind)
	}
	if err := validate(); err != nil {
		return zero, err
	}

	r.createMu.Lock()
	defer r.createMu.Unlock()

	if r.shutdown.Load() {
		return zero, ErrCollectorShutdown
	}

	// Another caller may have published the descriptor while we waited.
	if raw, ok := r.entries.Load(fullName); ok {
		if handle, ok := matchEntry[T](raw, desc); ok {
			return handle, nil
		}
		return zero, descriptorConflict(fullName, raw, desc.kind)
	}

	desc.labelNames = NormalizeLabelNames(desc.labelNames)
	desc.buckets = slices.Clone(desc.buckets)

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

func matchEntry[T any](raw any, requested descriptor) (T, bool) {
	if entry, ok := raw.(*metricEntry); ok && entry.desc.matches(requested) {
		handle, ok := entry.handle.(T)
		return handle, ok
	}
	var zero T
	return zero, false
}

func descriptorConflict(name string, raw any, kind instrumentKind) error {
	entry, _ := raw.(*metricEntry) //nolint:errcheck // an unexpected entry type is reported as unknown
	return fmt.Errorf("metric %q (existing %s, requested %s): %w", name, entryKindString(entry), kind, ErrDescriptorConflict)
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
