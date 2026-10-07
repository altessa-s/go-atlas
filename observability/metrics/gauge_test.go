// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

// publishGate parks the first publication of the value 1 until release is
// closed, so a test can run a second update while the first is in flight.
type publishGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newPublishGate() *publishGate {
	return &publishGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *publishGate) wait(value float64) {
	if value != 1 {
		return
	}
	g.once.Do(func() {
		close(g.entered)
		<-g.release
	})
}

// gatedMemoryAdapter gates the unbound RecordGauge path.
type gatedMemoryAdapter struct {
	*memory.Adapter
	gate *publishGate
}

func (a *gatedMemoryAdapter) RecordGauge(name string, labels map[string]string, value float64) {
	a.gate.wait(value)
	a.Adapter.RecordGauge(name, labels, value)
}

// gatedPromAdapter gates the bound (Binder) gauge path.
type gatedPromAdapter struct {
	*prometheusadapter.Adapter
	gate *publishGate
}

func (a *gatedPromAdapter) BindGauge(name string, labels map[string]string) (adapters.BoundGauge, bool) {
	h, ok := a.Adapter.BindGauge(name, labels)
	if !ok {
		return nil, false
	}
	return gatedGauge{inner: h, gate: a.gate}, true
}

type gatedGauge struct {
	inner adapters.BoundGauge
	gate  *publishGate
}

func (g gatedGauge) Set(value float64) {
	g.gate.wait(value)
	g.inner.Set(value)
}

// raceIncrements runs two Inc calls so that the second starts while the first
// is parked publishing the value 1. It is bounded concurrency coverage: on the
// old code the second Inc published 2 before the first published 1.
func raceIncrements(t *testing.T, g metrics.Gauge, gate *publishGate) {
	t.Helper()

	var wg sync.WaitGroup
	wg.Go(g.Inc)
	<-gate.entered

	secondDone := make(chan struct{})
	wg.Go(func() {
		defer close(secondDone)
		g.Inc()
	})
	select {
	case <-secondDone: // update and publication were not serialized
	case <-time.After(100 * time.Millisecond): // second Inc is waiting for the first
	}

	close(gate.release)
	wg.Wait()
}

func TestGauge_ConcurrentIncPublishesLatest_Unbound(t *testing.T) {
	t.Parallel()

	gate := newPublishGate()
	mem := memory.New()
	c := metrics.New(metrics.WithAdapter(&gatedMemoryAdapter{Adapter: mem, gate: gate}), metrics.WithServiceName("svc"))
	g := c.MustGauge(metrics.MetricOpts{Name: "depth", Help: "test"})

	raceIncrements(t, g, gate)

	require.InDelta(t, 2.0, mem.GaugeValue("svc_depth", nil), 0)
}

func TestGauge_ConcurrentIncPublishesLatest_Bound(t *testing.T) {
	t.Parallel()

	gate := newPublishGate()
	reg := prometheus.NewRegistry()
	adapter := &gatedPromAdapter{
		Adapter: prometheusadapter.New(prometheusadapter.WithRegisterer(reg), prometheusadapter.WithGatherer(reg)),
		gate:    gate,
	}
	c := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("svc"))
	g := c.MustGauge(metrics.MetricOpts{Name: "inflight", Help: "test", LabelNames: []string{"m"}}).
		WithLabels(metrics.Labels{"m": "GET"})

	raceIncrements(t, g, gate)

	got, _ := promValue(t, reg, "svc_inflight", map[string]string{"m": "GET"})
	require.InDelta(t, 2.0, got, 0)
}

func TestGauge_EquivalentLabelHandlesShareValue_Memory(t *testing.T) {
	t.Parallel()

	mem := memory.New()
	c := metrics.New(metrics.WithAdapter(mem), metrics.WithServiceName("svc"))
	g := c.MustGauge(metrics.MetricOpts{Name: "conns", Help: "test", LabelNames: []string{"a", "b"}})

	a := g.WithLabels(metrics.Labels{"a": "1", "b": "2"})
	b := g.WithLabels(metrics.Labels{"b": "2", "a": "1"})
	a.Inc()
	b.Inc()
	require.InDelta(t, 2.0, mem.GaugeValue("svc_conns", map[string]string{"a": "1", "b": "2"}), 0)

	// Nil and empty label sets address the unlabeled series.
	g.Inc()
	g.WithLabels(nil).Inc()
	g.WithLabels(metrics.Labels{}).Add(2)
	require.InDelta(t, 4.0, mem.GaugeValue("svc_conns", nil), 0)
}

func TestGauge_EquivalentLabelHandlesShareValue_Prometheus(t *testing.T) {
	t.Parallel()

	c, reg := newPromRegistryCollector(t)
	g := c.MustGauge(metrics.MetricOpts{Name: "conns", Help: "test", LabelNames: []string{"a"}})

	g.WithLabels(metrics.Labels{"a": "x"}).Inc()
	g.WithLabels(metrics.Labels{"a": "x"}).Inc()
	g.WithLabels(metrics.Labels{"a": "x"}).WithLabels(nil).Inc()

	got, _ := promValue(t, reg, "svc_conns", map[string]string{"a": "x"})
	require.InDelta(t, 3.0, got, 0)
}
