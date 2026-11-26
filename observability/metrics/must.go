// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

// RegisterCounter validates opts and registers or retrieves a [Counter] on m.
// Returns an error if opts are invalid, the metric name is already registered
// with an incompatible descriptor, the backend rejects registration, or the
// collector has been shut down.
func RegisterCounter(m Meter, opts MetricOpts) (Counter, error) {
	return m.RegisterCounter(opts)
}

// RegisterGauge validates opts and registers or retrieves a [Gauge] on m.
// Returns an error if opts are invalid, the metric name is already registered
// with an incompatible descriptor, the backend rejects registration, or the
// collector has been shut down.
func RegisterGauge(m Meter, opts MetricOpts) (Gauge, error) {
	return m.RegisterGauge(opts)
}

// RegisterHistogram validates opts and registers or retrieves a [Histogram] on m.
// Returns an error if opts are invalid, the metric name is already registered
// with an incompatible descriptor, the backend rejects registration, or the
// collector has been shut down.
func RegisterHistogram(m Meter, opts HistogramOpts) (Histogram, error) {
	return m.RegisterHistogram(opts)
}

// RegisterTimer validates opts and registers or retrieves a [Timer] on m.
// Returns an error if opts are invalid, the metric name is already registered
// with an incompatible descriptor, the backend rejects registration, or the
// collector has been shut down.
func RegisterTimer(m Meter, opts HistogramOpts) (Timer, error) {
	return m.RegisterTimer(opts)
}

// MustMeter adds validating registration methods to an embedded [Meter].
// Use it when registering several instruments on the same meter.
type MustMeter struct {
	Meter
}

// MustCounter validates opts and returns the meter's [Counter].
// It panics on invalid options, descriptor conflicts, or registration errors.
func MustCounter(m Meter, opts MetricOpts) Counter {
	c, err := RegisterCounter(m, opts)
	if err != nil {
		panic(err)
	}
	return c
}

// MustCounter registers a [Counter] through [MustCounter].
func (m MustMeter) MustCounter(opts MetricOpts) Counter {
	return MustCounter(m.Meter, opts)
}

// MustGauge validates opts and returns the meter's [Gauge].
// It panics on invalid options, descriptor conflicts, or registration errors.
func MustGauge(m Meter, opts MetricOpts) Gauge {
	g, err := RegisterGauge(m, opts)
	if err != nil {
		panic(err)
	}
	return g
}

// MustGauge registers a [Gauge] through [MustGauge].
func (m MustMeter) MustGauge(opts MetricOpts) Gauge {
	return MustGauge(m.Meter, opts)
}

// MustHistogram validates opts and returns the meter's [Histogram].
// It panics on invalid options, descriptor conflicts, or registration errors.
func MustHistogram(m Meter, opts HistogramOpts) Histogram {
	h, err := RegisterHistogram(m, opts)
	if err != nil {
		panic(err)
	}
	return h
}

// MustHistogram registers a [Histogram] through [MustHistogram].
func (m MustMeter) MustHistogram(opts HistogramOpts) Histogram {
	return MustHistogram(m.Meter, opts)
}

// MustTimer validates opts and returns the meter's [Timer].
// It panics on invalid options, descriptor conflicts, or registration errors.
func MustTimer(m Meter, opts HistogramOpts) Timer {
	tm, err := RegisterTimer(m, opts)
	if err != nil {
		panic(err)
	}
	return tm
}

// MustTimer registers a [Timer] through [MustTimer].
func (m MustMeter) MustTimer(opts HistogramOpts) Timer {
	return MustTimer(m.Meter, opts)
}
