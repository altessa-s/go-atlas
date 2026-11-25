// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

// MustMeter adds validating registration methods to an embedded [Meter].
// Use it when registering several instruments on the same meter.
type MustMeter struct {
	Meter
}

// MustCounter validates opts and returns the meter's [Counter]. It panics on invalid options.
func MustCounter(m Meter, opts MetricOpts) Counter {
	must(opts)
	return m.Counter(opts)
}

// MustCounter registers a [Counter] through [MustCounter].
func (m MustMeter) MustCounter(opts MetricOpts) Counter {
	return MustCounter(m.Meter, opts)
}

// MustGauge validates opts and returns the meter's [Gauge]. It panics on invalid options.
func MustGauge(m Meter, opts MetricOpts) Gauge {
	must(opts)
	return m.Gauge(opts)
}

// MustGauge registers a [Gauge] through [MustGauge].
func (m MustMeter) MustGauge(opts MetricOpts) Gauge {
	return MustGauge(m.Meter, opts)
}

// MustHistogram validates opts and returns the meter's [Histogram]. It panics on invalid options.
func MustHistogram(m Meter, opts HistogramOpts) Histogram {
	must(opts)
	return m.Histogram(opts)
}

// MustHistogram registers a [Histogram] through [MustHistogram].
func (m MustMeter) MustHistogram(opts HistogramOpts) Histogram {
	return MustHistogram(m.Meter, opts)
}

// MustTimer validates opts and returns the meter's [Timer]. It panics on invalid options.
func MustTimer(m Meter, opts HistogramOpts) Timer {
	must(opts)
	return m.Timer(opts)
}

// MustTimer registers a [Timer] through [MustTimer].
func (m MustMeter) MustTimer(opts HistogramOpts) Timer {
	return MustTimer(m.Meter, opts)
}

// must panics with the validation error for invalid metric options.
func must(opts interface{ Validate() error }) {
	if err := opts.Validate(); err != nil {
		panic(err)
	}
}
