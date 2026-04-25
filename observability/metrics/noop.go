// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"time"
)

// noopCollector is a no-op implementation of Collector.
// All operations are safe but do nothing.
type noopCollector struct{}

var noopInstance = &noopCollector{}

// Noop returns a no-op implementation of [Collector].
// All operations are safe but nothing is recorded.
// The returned value is a singleton; [IsNoop] can identify it.
//
// Use Noop() as the default when metrics are optional:
//
//	func newOptions(opts ...Option) *options {
//	    o := &options{
//	        metrics: metrics.Noop(), // Default to no-op
//	    }
//	    // ...
//	}
func Noop() Collector {
	return noopInstance
}

// IsNoop reports whether r is the singleton no-op [Collector] returned by [Noop].
func IsNoop(r Collector) bool {
	return r == noopInstance
}

// Counter implements Collector.
func (n *noopCollector) Counter(_ MetricOpts) Counter { return noopCounter{} }

// Gauge implements Collector.
func (n *noopCollector) Gauge(_ MetricOpts) Gauge { return noopGauge{} }

// Histogram implements Collector.
func (n *noopCollector) Histogram(_ HistogramOpts) Histogram { return noopHistogram{} }

// Timer implements Collector.
func (n *noopCollector) Timer(_ HistogramOpts) Timer { return noopTimer{} }

// MustCounter implements Collector.
func (n *noopCollector) MustCounter(opts MetricOpts) Counter { return noopCounter{} }

// MustGauge implements Collector.
func (n *noopCollector) MustGauge(_ MetricOpts) Gauge { return noopGauge{} }

// MustHistogram implements Collector.
func (n *noopCollector) MustHistogram(_ HistogramOpts) Histogram { return noopHistogram{} }

// MustTimer implements Collector.
func (n *noopCollector) MustTimer(_ HistogramOpts) Timer { return noopTimer{} }

// WithSubsystem implements Collector.
func (n *noopCollector) WithSubsystem(_ string) Collector { return n }

// Shutdown implements Collector.
func (n *noopCollector) Shutdown(_ context.Context) error { return nil }

// ForceFlush implements Collector.
func (n *noopCollector) ForceFlush(_ context.Context) error { return nil }

// Ensure noopCollector implements Collector.
var _ Collector = (*noopCollector)(nil)

// noopCounter is a no-op implementation of Counter.
type noopCounter struct{}

func (noopCounter) Inc()                        {}
func (noopCounter) Add(_ float64)               {}
func (noopCounter) WithLabels(_ Labels) Counter { return noopCounter{} }

// noopGauge is a no-op implementation of Gauge.
type noopGauge struct{}

func (noopGauge) Set(_ float64)             {}
func (noopGauge) Inc()                      {}
func (noopGauge) Dec()                      {}
func (noopGauge) Add(_ float64)             {}
func (noopGauge) Sub(_ float64)             {}
func (noopGauge) WithLabels(_ Labels) Gauge { return noopGauge{} }

// noopHistogram is a no-op implementation of Histogram.
type noopHistogram struct{}

func (noopHistogram) Observe(_ float64)             {}
func (noopHistogram) WithLabels(_ Labels) Histogram { return noopHistogram{} }

// noopTimer is a no-op implementation of Timer.
type noopTimer struct{}

func (noopTimer) Start() func()                   { return func() {} }
func (noopTimer) ObserveDuration(_ time.Duration) {}
func (noopTimer) WithLabels(_ Labels) Timer       { return noopTimer{} }
