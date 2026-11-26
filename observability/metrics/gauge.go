// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// gaugeSeries is the shadow value of one gauge series for backends that only
// accept absolute gauge values. mu serializes each update with its publication
// so the backend always ends with the value of the last update.
type gaugeSeries struct {
	mu    sync.Mutex
	value float64
}

// gauge is the default implementation of Gauge.
type gauge struct {
	name       string
	adapter    adapters.Adapter
	gaugeAdder adapters.GaugeAdder
	shutdown   *atomic.Bool
	unlabeled  gaugeSeries
	bound      adapters.BoundGauge      // unlabeled child, bound at registration when supported
	boundAdder adapters.BoundGaugeAdder // non-nil when bound also supports native Add
	series     sync.Map                 // seriesKey(labels) -> *gaugeSeries
}

// bindUnlabeled resolves the unlabeled child once. The collector calls it
// after registration and before publishing the handle.
func (g *gauge) bindUnlabeled() {
	if b, ok := g.adapter.(adapters.GaugeBinder); ok {
		if h, ok := b.BindGauge(g.name, nil); ok {
			g.bound = h
			if ba, ok := h.(adapters.BoundGaugeAdder); ok {
				g.boundAdder = ba
			}
		}
	}
}

// publishUnlabeled records the unlabeled value. Caller holds g.unlabeled.mu.
func (g *gauge) publishUnlabeled() {
	if g.bound != nil {
		g.bound.Set(g.unlabeled.value)
		return
	}
	g.adapter.RecordGauge(g.name, nil, g.unlabeled.value)
}

func newGauge(name string, adapter adapters.Adapter, shutdown *atomic.Bool) *gauge {
	if shutdown == nil {
		shutdown = &activeLifecycle
	}
	ga, _ := adapter.(adapters.GaugeAdder) //nolint:errcheck // optional capability
	return &gauge{
		name:       name,
		adapter:    adapter,
		gaugeAdder: ga,
		shutdown:   shutdown,
	}
}

func (g *gauge) Set(value float64) {
	if g.shutdown.Load() {
		return
	}
	if g.boundAdder != nil || g.gaugeAdder != nil {
		if g.bound != nil {
			g.bound.Set(value)
		} else {
			g.adapter.RecordGauge(g.name, nil, value)
		}
		return
	}
	g.unlabeled.mu.Lock()
	defer g.unlabeled.mu.Unlock()
	g.unlabeled.value = value
	g.publishUnlabeled()
}

func (g *gauge) Inc() {
	g.Add(1)
}

func (g *gauge) Dec() {
	g.Add(-1)
}

func (g *gauge) Add(delta float64) {
	if g.shutdown.Load() {
		return
	}
	if g.boundAdder != nil {
		g.boundAdder.Add(delta)
		return
	}
	if g.gaugeAdder != nil {
		g.gaugeAdder.AddGauge(g.name, nil, delta)
		return
	}
	g.unlabeled.mu.Lock()
	defer g.unlabeled.mu.Unlock()
	g.unlabeled.value += delta
	g.publishUnlabeled()
}

func (g *gauge) Sub(delta float64) {
	g.Add(-delta)
}

func (g *gauge) WithLabels(labels Labels) Gauge {
	if g.shutdown.Load() {
		return noopGauge{}
	}
	return newLabeledGauge(g, labels)
}

// seriesFor returns the shadow state shared by every handle of the given
// label set. An empty label set addresses the unlabeled series.
func (g *gauge) seriesFor(labels Labels) *gaugeSeries {
	if len(labels) == 0 {
		return &g.unlabeled
	}
	key := seriesKey(labels)
	if existing, ok := g.series.Load(key); ok {
		if s, ok := existing.(*gaugeSeries); ok {
			return s
		}
	}
	actual, _ := g.series.LoadOrStore(key, &gaugeSeries{})
	s, ok := actual.(*gaugeSeries)
	if !ok {
		return &gaugeSeries{}
	}
	return s
}

// seriesKey encodes a label set canonically: names sorted, each name and
// value length-prefixed so arbitrary characters cannot make two different
// label sets collide.
func seriesKey(labels Labels) string {
	var b strings.Builder
	for _, k := range SortedKeys(labels) {
		v := labels[k]
		b.WriteString(strconv.Itoa(len(k)))
		b.WriteByte(':')
		b.WriteString(k)
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
	}
	return b.String()
}

// newLabeledGauge builds a labeled view.
// When the backend supports native additive updates ([adapters.BoundGaugeAdder]
// or [adapters.GaugeAdder]), updates go directly to the backend; otherwise
// handles share per-series shadow state.
func newLabeledGauge(parent *gauge, labels Labels) *labeledGauge {
	var (
		bound      adapters.BoundGauge
		boundAdder adapters.BoundGaugeAdder
	)
	if b, ok := parent.adapter.(adapters.GaugeBinder); ok {
		if h, ok := b.BindGauge(parent.name, labels); ok {
			bound = h
			if ba, ok := h.(adapters.BoundGaugeAdder); ok {
				boundAdder = ba
			}
		}
	}
	needShadow := boundAdder == nil && parent.gaugeAdder == nil
	var series *gaugeSeries
	if needShadow {
		series = parent.seriesFor(labels)
	}
	return &labeledGauge{
		parent:     parent,
		shutdown:   parent.shutdown,
		labels:     retainLabels(labels, bound == nil || (boundAdder == nil && parent.gaugeAdder != nil)),
		series:     series,
		bound:      bound,
		boundAdder: boundAdder,
	}
}

// labeledGauge is a gauge with labels applied.
type labeledGauge struct {
	parent     *gauge
	shutdown   *atomic.Bool
	labels     Labels
	series     *gaugeSeries
	bound      adapters.BoundGauge      // non-nil when the adapter pre-resolved the child
	boundAdder adapters.BoundGaugeAdder // non-nil when bound also supports native Add
}

func (l *labeledGauge) Set(value float64) {
	if l.shutdown.Load() {
		return
	}
	if l.series == nil {
		l.record(value)
		return
	}
	l.series.mu.Lock()
	defer l.series.mu.Unlock()
	l.series.value = value
	l.record(value)
}

func (l *labeledGauge) Inc() {
	l.Add(1)
}

func (l *labeledGauge) Dec() {
	l.Add(-1)
}

func (l *labeledGauge) Add(delta float64) {
	if l.shutdown.Load() {
		return
	}
	if l.boundAdder != nil {
		l.boundAdder.Add(delta)
		return
	}
	if l.parent.gaugeAdder != nil {
		l.parent.gaugeAdder.AddGauge(l.parent.name, l.labels, delta)
		return
	}
	l.series.mu.Lock()
	defer l.series.mu.Unlock()
	l.series.value += delta
	l.record(l.series.value)
}

func (l *labeledGauge) Sub(delta float64) {
	l.Add(-delta)
}

// record publishes value. Shadow gauge callers hold l.series.mu.
func (l *labeledGauge) record(value float64) {
	if l.bound != nil {
		l.bound.Set(value)
		return
	}
	l.parent.adapter.RecordGauge(l.parent.name, l.labels, value)
}

func (l *labeledGauge) WithLabels(labels Labels) Gauge {
	if l.shutdown.Load() {
		return noopGauge{}
	}
	return newLabeledGauge(l.parent, MergeLabels(l.labels, labels))
}
