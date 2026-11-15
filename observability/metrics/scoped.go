// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "github.com/altessa-s/go-atlas/observability/internal/shared"

// scopedMeter is a Meter that adds a subsystem prefix to all metrics.
// It deliberately does not implement Collector: the lifecycle belongs to
// the parent collector's owner, not to the component holding the scope.
type scopedMeter struct {
	parent    *collector
	subsystem string
}

// Counter implements Meter.
func (s *scopedMeter) Counter(opts MetricOpts) Counter {
	return s.parent.getOrCreateCounter(s.subsystem, opts)
}

// Gauge implements Meter.
func (s *scopedMeter) Gauge(opts MetricOpts) Gauge {
	return s.parent.getOrCreateGauge(s.subsystem, opts)
}

// Histogram implements Meter.
func (s *scopedMeter) Histogram(opts HistogramOpts) Histogram {
	return s.parent.getOrCreateHistogram(s.subsystem, opts)
}

// Timer implements Meter.
func (s *scopedMeter) Timer(opts HistogramOpts) Timer {
	return s.parent.getOrCreateTimer(s.subsystem, opts)
}

// MustCounter implements Meter.
func (s *scopedMeter) MustCounter(opts MetricOpts) Counter {
	return must(opts, func() Counter { return s.Counter(opts) })
}

// MustGauge implements Meter.
func (s *scopedMeter) MustGauge(opts MetricOpts) Gauge {
	return must(opts, func() Gauge { return s.Gauge(opts) })
}

// MustHistogram implements Meter.
func (s *scopedMeter) MustHistogram(opts HistogramOpts) Histogram {
	return must(opts, func() Histogram { return s.Histogram(opts) })
}

// MustTimer implements Meter.
func (s *scopedMeter) MustTimer(opts HistogramOpts) Timer {
	return must(opts, func() Timer { return s.Timer(opts) })
}

// WithSubsystem implements Meter.
// Nested subsystems are joined with underscores.
func (s *scopedMeter) WithSubsystem(subsystem string) Meter {
	return &scopedMeter{
		parent:    s.parent,
		subsystem: shared.JoinMetricScope(s.subsystem, subsystem),
	}
}

// Ensure scopedMeter implements Meter.
var _ Meter = (*scopedMeter)(nil)
