// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestNoopCollector_Counter(t *testing.T) {
	c := metrics.Noop().Counter(metrics.MetricOpts{Name: "test"})
	c.Inc()
	c.Add(5)
	c2 := c.WithLabels(metrics.Labels{"k": "v"})
	c2.Inc()
}

func TestNoopCollector_Gauge(t *testing.T) {
	g := metrics.Noop().Gauge(metrics.MetricOpts{Name: "test"})
	g.Set(1)
	g.Inc()
	g.Dec()
	g.Add(5)
	g.Sub(3)
	g2 := g.WithLabels(metrics.Labels{"k": "v"})
	g2.Set(0)
}

func TestNoopCollector_Histogram(t *testing.T) {
	h := metrics.Noop().Histogram(metrics.HistogramOpts{MetricOpts: metrics.MetricOpts{Name: "test"}})
	h.Observe(1.5)
	h2 := h.WithLabels(metrics.Labels{"k": "v"})
	h2.Observe(2.5)
}

func TestNoopCollector_Timer(t *testing.T) {
	tm := metrics.Noop().Timer(metrics.HistogramOpts{MetricOpts: metrics.MetricOpts{Name: "test"}})
	stop := tm.Start()
	stop()
	tm.ObserveDuration(100 * time.Millisecond)
	tm2 := tm.WithLabels(metrics.Labels{"k": "v"})
	tm2.ObserveDuration(50 * time.Millisecond)
}

func TestNoopCollector_MustVariants(t *testing.T) {
	n := metrics.Noop()
	_ = n.MustCounter(metrics.MetricOpts{Name: "c"})
	_ = n.MustGauge(metrics.MetricOpts{Name: "g"})
	_ = n.MustHistogram(metrics.HistogramOpts{MetricOpts: metrics.MetricOpts{Name: "h"}})
	_ = n.MustTimer(metrics.HistogramOpts{MetricOpts: metrics.MetricOpts{Name: "t"}})
}

func TestNoopCollector_WithSubsystem(t *testing.T) {
	n := metrics.Noop()
	sub := n.WithSubsystem("test")
	if !metrics.IsNoop(sub) {
		t.Error("WithSubsystem should return noop")
	}
}

func TestNoopCollector_Shutdown(t *testing.T) {
	if err := metrics.Noop().Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNoopCollector_ForceFlush(t *testing.T) {
	if err := metrics.Noop().ForceFlush(t.Context()); err != nil {
		t.Errorf("ForceFlush() error = %v", err)
	}
}

func TestGetLabels_Pool(t *testing.T) {
	l := metrics.GetLabels()
	if l == nil {
		t.Fatal("GetLabels() returned nil")
	}
	(*l)["key"] = "val"
	metrics.PutLabels(l)
}

func TestGetLabelsWithCapacity(t *testing.T) {
	l := metrics.GetLabelsWithCapacity(16)
	if l == nil {
		t.Fatal("GetLabelsWithCapacity() returned nil")
	}
	metrics.PutLabels(l)
}

func TestLabelsKeys(t *testing.T) {
	l := metrics.Labels{"a": "1", "b": "2"}
	count := 0
	for range metrics.LabelsKeys(l) {
		count++
	}
	if count != 2 {
		t.Errorf("LabelsKeys count = %d, want 2", count)
	}
}

func TestLabelsValues(t *testing.T) {
	l := metrics.Labels{"a": "1", "b": "2"}
	count := 0
	for range metrics.LabelsValues(l) {
		count++
	}
	if count != 2 {
		t.Errorf("LabelsValues count = %d, want 2", count)
	}
}
