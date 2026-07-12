// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"math"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// atomicAddFloat64 atomically adds delta to the value stored in v and returns the new value.
// Uses CAS loop with float64 bits representation.
func atomicAddFloat64(v *atomic.Uint64, delta float64) float64 {
	for {
		old := v.Load()
		newValue := math.Float64frombits(old) + delta
		if v.CompareAndSwap(old, math.Float64bits(newValue)) {
			return newValue
		}
	}
}

// gauge is the default implementation of Gauge with atomic operations.
type gauge struct {
	name       string
	labelNames []string
	adapter    adapters.Adapter
	value      atomic.Uint64 // stores float64 bits atomically
}

func newGauge(name string, labelNames []string, adapter adapters.Adapter) *gauge {
	return &gauge{
		name:       name,
		labelNames: labelNames,
		adapter:    adapter,
	}
}

func (g *gauge) Set(value float64) {
	g.value.Store(math.Float64bits(value))
	g.adapter.RecordGauge(g.name, nil, value)
}

func (g *gauge) Inc() {
	g.Add(1)
}

func (g *gauge) Dec() {
	g.Add(-1)
}

func (g *gauge) Add(delta float64) {
	newValue := atomicAddFloat64(&g.value, delta)
	g.adapter.RecordGauge(g.name, nil, newValue)
}

func (g *gauge) Sub(delta float64) {
	g.Add(-delta)
}

func (g *gauge) WithLabels(labels Labels) Gauge {
	return newLabeledGauge(g, labels)
}

// newLabeledGauge builds a labeled view and, when the adapter supports
// [adapters.Binder], resolves the concrete child once so Set/Add are direct
// updates with no per-observation lookup. The local shadow value is kept so
// Add/Inc/Dec semantics stay identical to the unbound path.
func newLabeledGauge(parent *gauge, labels Labels) *labeledGauge {
	l := &labeledGauge{parent: parent, labels: labels}
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
	bound  adapters.BoundGauge // non-nil when the adapter pre-resolved the child
	value  atomic.Uint64
}

func (l *labeledGauge) Set(value float64) {
	l.value.Store(math.Float64bits(value))
	l.record(value)
}

func (l *labeledGauge) Inc() {
	l.Add(1)
}

func (l *labeledGauge) Dec() {
	l.Add(-1)
}

func (l *labeledGauge) Add(delta float64) {
	l.record(atomicAddFloat64(&l.value, delta))
}

func (l *labeledGauge) Sub(delta float64) {
	l.Add(-delta)
}

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
