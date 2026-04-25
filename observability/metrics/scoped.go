// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"

	"github.com/altessa-s/go-atlas/observability/internal/shared"
)

// scopedCollector is a Collector that adds a subsystem prefix to all metrics.
type scopedCollector struct {
	parent    *collector
	subsystem string
}

// Counter implements Collector.
func (s *scopedCollector) Counter(opts MetricOpts) Counter {
	return s.parent.getOrCreateCounter(s.subsystem, opts)
}

// Gauge implements Collector.
func (s *scopedCollector) Gauge(opts MetricOpts) Gauge {
	return s.parent.getOrCreateGauge(s.subsystem, opts)
}

// Histogram implements Collector.
func (s *scopedCollector) Histogram(opts HistogramOpts) Histogram {
	return s.parent.getOrCreateHistogram(s.subsystem, opts)
}

// Timer implements Collector.
func (s *scopedCollector) Timer(opts HistogramOpts) Timer {
	return s.parent.getOrCreateTimer(s.subsystem, opts)
}

// MustCounter implements Collector.
func (s *scopedCollector) MustCounter(opts MetricOpts) Counter {
	return must(opts, func() Counter { return s.Counter(opts) })
}

// MustGauge implements Collector.
func (s *scopedCollector) MustGauge(opts MetricOpts) Gauge {
	return must(opts, func() Gauge { return s.Gauge(opts) })
}

// MustHistogram implements Collector.
func (s *scopedCollector) MustHistogram(opts HistogramOpts) Histogram {
	return must(opts, func() Histogram { return s.Histogram(opts) })
}

// MustTimer implements Collector.
func (s *scopedCollector) MustTimer(opts HistogramOpts) Timer {
	return must(opts, func() Timer { return s.Timer(opts) })
}

// WithSubsystem implements Collector.
// Nested subsystems are joined with underscores.
func (s *scopedCollector) WithSubsystem(subsystem string) Collector {
	return &scopedCollector{
		parent:    s.parent,
		subsystem: shared.JoinMetricScope(s.subsystem, subsystem),
	}
}

// Shutdown implements Collector.
func (s *scopedCollector) Shutdown(ctx context.Context) error {
	return s.parent.Shutdown(ctx)
}

// ForceFlush implements Collector.
func (s *scopedCollector) ForceFlush(ctx context.Context) error {
	return s.parent.ForceFlush(ctx)
}

// Ensure scopedCollector implements Collector.
var _ Collector = (*scopedCollector)(nil)
