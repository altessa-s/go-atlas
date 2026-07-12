// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// counter is the default implementation of Counter with atomic operations.
type counter struct {
	name       string
	labelNames []string
	adapter    adapters.Adapter
	value      atomic.Uint64
}

func newCounter(name string, labelNames []string, adapter adapters.Adapter) *counter {
	return &counter{
		name:       name,
		labelNames: labelNames,
		adapter:    adapter,
	}
}

func (c *counter) Inc() {
	c.Add(1)
}

func (c *counter) Add(delta float64) {
	if delta < 0 {
		return // Counters can only increase
	}
	atomicAddFloat64(&c.value, delta)
	c.adapter.RecordCounter(c.name, nil, delta)
}

func (c *counter) WithLabels(labels Labels) Counter {
	return newLabeledCounter(c, labels)
}

// newLabeledCounter builds a labeled view and, when the adapter supports
// [adapters.Binder], resolves the concrete child once so Inc/Add are direct
// updates with no per-observation lookup.
func newLabeledCounter(parent *counter, labels Labels) *labeledCounter {
	l := &labeledCounter{parent: parent, labels: labels}
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindCounter(parent.name, labels); ok {
			l.bound = h
		}
	}
	return l
}

// labeledCounter is a counter with labels applied.
type labeledCounter struct {
	parent *counter
	labels Labels
	bound  adapters.BoundCounter // non-nil when the adapter pre-resolved the child
}

func (l *labeledCounter) Inc() {
	l.Add(1)
}

func (l *labeledCounter) Add(delta float64) {
	if delta < 0 {
		return
	}
	if l.bound != nil {
		l.bound.Add(delta)
		return
	}
	l.parent.adapter.RecordCounter(l.parent.name, l.labels, delta)
}

func (l *labeledCounter) WithLabels(labels Labels) Counter {
	return newLabeledCounter(l.parent, MergeLabels(l.labels, labels))
}
