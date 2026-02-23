// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"
	"time"
)

func TestNoop_ReturnsSingleton(t *testing.T) {
	a := Noop()
	b := Noop()
	if a != b {
		t.Fatal("Noop() should return the same instance")
	}
}

func TestIsNoop(t *testing.T) {
	tests := []struct {
		name string
		c    Collector
		want bool
	}{
		{"noop collector", Noop(), true},
		{"new without adapter", New(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNoop(tt.c); got != tt.want {
				t.Errorf("IsNoop() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoopCollector_MethodsDoNotPanic(t *testing.T) {
	c := Noop()

	counter := c.Counter(MetricOpts{Name: "test"})
	counter.Inc()
	counter.Add(5)
	counter.WithLabels(Labels{"k": "v"}).Inc()

	gauge := c.Gauge(MetricOpts{Name: "test"})
	gauge.Set(1)
	gauge.Inc()
	gauge.Dec()
	gauge.Add(1)
	gauge.Sub(1)
	gauge.WithLabels(Labels{"k": "v"}).Set(0)

	hist := c.Histogram(HistogramOpts{MetricOpts: MetricOpts{Name: "test"}})
	hist.Observe(1.0)
	hist.WithLabels(Labels{"k": "v"}).Observe(2.0)

	timer := c.Timer(HistogramOpts{MetricOpts: MetricOpts{Name: "test"}})
	stop := timer.Start()
	stop()
	timer.ObserveDuration(time.Millisecond)
	timer.WithLabels(Labels{"k": "v"}).ObserveDuration(time.Second)

	// Must* variants
	c.MustCounter(MetricOpts{Name: "test"}).Inc()
	c.MustGauge(MetricOpts{Name: "test"}).Set(0)
	c.MustHistogram(HistogramOpts{MetricOpts: MetricOpts{Name: "test"}}).Observe(1)
	c.MustTimer(HistogramOpts{MetricOpts: MetricOpts{Name: "test"}}).ObserveDuration(time.Second)

	// WithSubsystem returns same noop
	sub := c.WithSubsystem("sub")
	if !IsNoop(sub) {
		t.Error("WithSubsystem on noop should return noop")
	}

	// Shutdown and ForceFlush
	if err := c.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() = %v", err)
	}
	if err := c.ForceFlush(t.Context()); err != nil {
		t.Errorf("ForceFlush() = %v", err)
	}
}
