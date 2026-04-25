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
	return &labeledGauge{
		parent: g,
		labels: labels,
	}
}

// labeledGauge is a gauge with labels applied.
type labeledGauge struct {
	parent *gauge
	labels Labels
	value  atomic.Uint64
}

func (l *labeledGauge) Set(value float64) {
	l.value.Store(math.Float64bits(value))
	l.parent.adapter.RecordGauge(l.parent.name, l.labels, value)
}

func (l *labeledGauge) Inc() {
	l.Add(1)
}

func (l *labeledGauge) Dec() {
	l.Add(-1)
}

func (l *labeledGauge) Add(delta float64) {
	newValue := atomicAddFloat64(&l.value, delta)
	l.parent.adapter.RecordGauge(l.parent.name, l.labels, newValue)
}

func (l *labeledGauge) Sub(delta float64) {
	l.Add(-delta)
}

func (l *labeledGauge) WithLabels(labels Labels) Gauge {
	return &labeledGauge{
		parent: l.parent,
		labels: MergeLabels(l.labels, labels),
	}
}
