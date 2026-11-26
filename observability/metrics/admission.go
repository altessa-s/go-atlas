// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// admission tracks backend calls for adapters with a flush or close phase.
// After the collector closes admission, taking the write lock drains the
// calls that passed the shutdown check under a read lock.
type admission struct {
	mu     sync.RWMutex
	closed *atomic.Bool
}

func (a *admission) enter() bool {
	a.mu.RLock()
	if a.closed.Load() {
		a.mu.RUnlock()
		return false
	}
	return true
}

func (a *admission) record(update func()) {
	if a.enter() {
		defer a.mu.RUnlock()
		update()
	}
}

type guardedAdapter struct {
	adapters.Adapter
	gate *admission
}

func (a *guardedAdapter) RecordCounter(name string, labels map[string]string, delta float64) {
	a.gate.record(func() { a.Adapter.RecordCounter(name, labels, delta) })
}

func (a *guardedAdapter) RecordGauge(name string, labels map[string]string, value float64) {
	a.gate.record(func() { a.Adapter.RecordGauge(name, labels, value) })
}

func (a *guardedAdapter) RecordHistogram(name string, labels map[string]string, value float64) {
	a.gate.record(func() { a.Adapter.RecordHistogram(name, labels, value) })
}

func (a *guardedAdapter) BindCounter(name string, labels map[string]string) (adapters.BoundCounter, bool) {
	binder, ok := a.Adapter.(adapters.Binder)
	if !ok || !a.gate.enter() {
		return nil, false
	}
	defer a.gate.mu.RUnlock()
	child, ok := binder.BindCounter(name, labels)
	if !ok {
		return nil, false
	}
	return guardedCounter{child: child, gate: a.gate}, true
}

func (a *guardedAdapter) BindHistogram(name string, labels map[string]string) (adapters.BoundHistogram, bool) {
	binder, ok := a.Adapter.(adapters.Binder)
	if !ok || !a.gate.enter() {
		return nil, false
	}
	defer a.gate.mu.RUnlock()
	child, ok := binder.BindHistogram(name, labels)
	if !ok {
		return nil, false
	}
	return guardedHistogram{child: child, gate: a.gate}, true
}

func (a *guardedAdapter) BindGauge(name string, labels map[string]string) (adapters.BoundGauge, bool) {
	binder, ok := a.Adapter.(adapters.GaugeBinder)
	if !ok || !a.gate.enter() {
		return nil, false
	}
	defer a.gate.mu.RUnlock()
	child, ok := binder.BindGauge(name, labels)
	if !ok {
		return nil, false
	}
	guarded := guardedGauge{child: child, gate: a.gate}
	if adder, ok := child.(adapters.BoundGaugeAdder); ok {
		return guardedGaugeAdder{guardedGauge: guarded, adder: adder}, true
	}
	return guarded, true
}

type guardedAdapterAdder struct {
	*guardedAdapter
	adder adapters.GaugeAdder
}

func (a *guardedAdapterAdder) AddGauge(name string, labels map[string]string, delta float64) {
	a.gate.record(func() { a.adder.AddGauge(name, labels, delta) })
}

type guardedCounter struct {
	child adapters.BoundCounter
	gate  *admission
}

func (c guardedCounter) Add(delta float64) {
	c.gate.record(func() { c.child.Add(delta) })
}

type guardedHistogram struct {
	child adapters.BoundHistogram
	gate  *admission
}

func (h guardedHistogram) Observe(value float64) {
	h.gate.record(func() { h.child.Observe(value) })
}

type guardedGauge struct {
	child adapters.BoundGauge
	gate  *admission
}

func (g guardedGauge) Set(value float64) {
	g.gate.record(func() { g.child.Set(value) })
}

type guardedGaugeAdder struct {
	guardedGauge
	adder adapters.BoundGaugeAdder
}

func (g guardedGaugeAdder) Add(delta float64) {
	g.gate.record(func() { g.adder.Add(delta) })
}
