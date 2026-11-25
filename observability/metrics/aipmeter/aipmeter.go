// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package aipmeter

import (
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"

	aipmetrics "github.com/altessa-s/go-aip/metrics"
)

// New returns a go-aip [aipmetrics.Meter] that records into m. The counters
// and timers it creates are those of m, so the go-aip parsers and translators
// record under m's namespace and subsystem. A nil m (typed nil included)
// returns [aipmetrics.Noop].
func New(m metrics.Meter) aipmetrics.Meter {
	if nilcheck.IsNil(m) {
		return aipmetrics.Noop()
	}

	return meter{m: m}
}

// meter forwards the go-aip instrument calls to a go-atlas Meter.
type meter struct {
	m metrics.Meter
}

// Counter creates a counter on the wrapped Meter.
func (a meter) Counter(opts aipmetrics.MetricOpts) aipmetrics.Counter {
	return a.m.Counter(metricOpts(opts))
}

// Timer creates a timer on the wrapped Meter, with its default buckets.
func (a meter) Timer(opts aipmetrics.HistogramOpts) aipmetrics.Timer {
	return a.m.Timer(histogramOpts(opts))
}

// MustCounter creates a counter on the wrapped Meter, panicking on error.
func (a meter) MustCounter(opts aipmetrics.MetricOpts) aipmetrics.Counter {
	return metrics.MustCounter(a.m, metricOpts(opts))
}

// MustTimer creates a timer on the wrapped Meter, panicking on error.
func (a meter) MustTimer(opts aipmetrics.HistogramOpts) aipmetrics.Timer {
	return metrics.MustTimer(a.m, histogramOpts(opts))
}

// WithSubsystem scopes the wrapped Meter to subsystem.
func (a meter) WithSubsystem(subsystem string) aipmetrics.Meter {
	return meter{m: a.m.WithSubsystem(subsystem)}
}

// metricOpts converts go-aip counter options.
func metricOpts(o aipmetrics.MetricOpts) metrics.MetricOpts {
	return metrics.MetricOpts{Name: o.Name, Help: o.Help}
}

// histogramOpts converts go-aip timer options; the buckets are left to the
// go-atlas collector.
func histogramOpts(o aipmetrics.HistogramOpts) metrics.HistogramOpts {
	return metrics.HistogramOpts{MetricOpts: metricOpts(o.MetricOpts)}
}
