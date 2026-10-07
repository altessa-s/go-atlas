// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"strconv"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// gaugeSeries is the shadow value of one gauge series. Adapters only accept
// absolute gauge values, so Add is computed locally; mu serializes each
// update with its publication so the backend always ends with the value of
// the last update.
type gaugeSeries struct {
	mu    sync.Mutex
	value float64
}

// gauge is the default implementation of Gauge.
type gauge struct {
	name      string
	adapter   adapters.Adapter
	unlabeled gaugeSeries
	series    sync.Map // seriesKey(labels) → *gaugeSeries, shared by all equivalent handles
}

func newGauge(name string, adapter adapters.Adapter) *gauge {
	return &gauge{
		name:    name,
		adapter: adapter,
	}
}

func (g *gauge) Set(value float64) {
	g.unlabeled.mu.Lock()
	defer g.unlabeled.mu.Unlock()
	g.unlabeled.value = value
	g.adapter.RecordGauge(g.name, nil, value)
}

func (g *gauge) Inc() {
	g.Add(1)
}

func (g *gauge) Dec() {
	g.Add(-1)
}

func (g *gauge) Add(delta float64) {
	g.unlabeled.mu.Lock()
	defer g.unlabeled.mu.Unlock()
	g.unlabeled.value += delta
	g.adapter.RecordGauge(g.name, nil, g.unlabeled.value)
}

func (g *gauge) Sub(delta float64) {
	g.Add(-delta)
}

func (g *gauge) WithLabels(labels Labels) Gauge {
	return newLabeledGauge(g, labels)
}

// seriesFor returns the shadow state shared by every handle of the given
// label set. An empty label set addresses the unlabeled series.
func (g *gauge) seriesFor(labels Labels) *gaugeSeries {
	if len(labels) == 0 {
		return &g.unlabeled
	}
	key := seriesKey(labels)
	if s, ok := g.series.Load(key); ok {
		return s.(*gaugeSeries) //nolint:errcheck // type guaranteed by LoadOrStore
	}
	s, _ := g.series.LoadOrStore(key, &gaugeSeries{})
	return s.(*gaugeSeries) //nolint:errcheck // type guaranteed by LoadOrStore
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

// newLabeledGauge builds a labeled view bound to the series state shared by
// all equivalent handles and, when the adapter supports [adapters.Binder],
// resolves the concrete child once so Set/Add are direct updates with no
// per-observation lookup.
func newLabeledGauge(parent *gauge, labels Labels) *labeledGauge {
	l := &labeledGauge{parent: parent, labels: labels, series: parent.seriesFor(labels)}
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindGauge(parent.name, labels); ok {
			l.bound = h
		}
	}
	return l
}

// labeledGauge is a gauge with labels applied.
type labeledGauge struct {
	parent *gauge
	labels Labels
	series *gaugeSeries
	bound  adapters.BoundGauge // non-nil when the adapter pre-resolved the child
}

func (l *labeledGauge) Set(value float64) {
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
	l.series.mu.Lock()
	defer l.series.mu.Unlock()
	l.series.value += delta
	l.record(l.series.value)
}

func (l *labeledGauge) Sub(delta float64) {
	l.Add(-delta)
}

// record publishes value; callers hold l.series.mu.
func (l *labeledGauge) record(value float64) {
	if l.bound != nil {
		l.bound.Set(value)
		return
	}
	l.parent.adapter.RecordGauge(l.parent.name, l.labels, value)
}

func (l *labeledGauge) WithLabels(labels Labels) Gauge {
	return newLabeledGauge(l.parent, MergeLabels(l.labels, labels))
}
