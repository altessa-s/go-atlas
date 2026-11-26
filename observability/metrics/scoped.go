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
	c, err := s.parent.tryGetOrCreateCounter(s.subsystem, opts)
	if err != nil {
		s.parent.recordError(shared.BuildMetricName(s.parent.serviceName, s.subsystem, opts.Name), kindCounter, err)
		return noopCounter{}
	}
	return c
}

// Gauge implements Meter.
func (s *scopedMeter) Gauge(opts MetricOpts) Gauge {
	g, err := s.parent.tryGetOrCreateGauge(s.subsystem, opts)
	if err != nil {
		s.parent.recordError(shared.BuildMetricName(s.parent.serviceName, s.subsystem, opts.Name), kindGauge, err)
		return noopGauge{}
	}
	return g
}

// Histogram implements Meter.
func (s *scopedMeter) Histogram(opts HistogramOpts) Histogram {
	h, err := s.parent.tryGetOrCreateHistogram(s.subsystem, opts)
	if err != nil {
		s.parent.recordError(shared.BuildMetricName(s.parent.serviceName, s.subsystem, opts.Name), kindHistogram, err)
		return noopHistogram{}
	}
	return h
}

// Timer implements Meter.
func (s *scopedMeter) Timer(opts HistogramOpts) Timer {
	tm, err := s.parent.tryGetOrCreateTimer(s.subsystem, opts)
	if err != nil {
		s.parent.recordError(shared.BuildMetricName(s.parent.serviceName, s.subsystem, opts.Name), kindTimer, err)
		return noopTimer{}
	}
	return tm
}

// RegisterCounter implements Meter.
func (s *scopedMeter) RegisterCounter(opts MetricOpts) (Counter, error) {
	return s.parent.tryGetOrCreateCounter(s.subsystem, opts)
}

// RegisterGauge implements Meter.
func (s *scopedMeter) RegisterGauge(opts MetricOpts) (Gauge, error) {
	return s.parent.tryGetOrCreateGauge(s.subsystem, opts)
}

// RegisterHistogram implements Meter.
func (s *scopedMeter) RegisterHistogram(opts HistogramOpts) (Histogram, error) {
	return s.parent.tryGetOrCreateHistogram(s.subsystem, opts)
}

// RegisterTimer implements Meter.
func (s *scopedMeter) RegisterTimer(opts HistogramOpts) (Timer, error) {
	return s.parent.tryGetOrCreateTimer(s.subsystem, opts)
}

// WithSubsystem implements Meter.
// Nested subsystems are joined with underscores.
func (s *scopedMeter) WithSubsystem(subsystem string) Meter {
	return &scopedMeter{
		parent:    s.parent,
		subsystem: shared.JoinMetricScope(s.subsystem, subsystem),
	}
}

var _ Meter = (*scopedMeter)(nil)
