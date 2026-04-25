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
	return &labeledCounter{
		parent: c,
		labels: labels,
	}
}

// labeledCounter is a counter with labels applied.
type labeledCounter struct {
	parent *counter
	labels Labels
}

func (l *labeledCounter) Inc() {
	l.Add(1)
}

func (l *labeledCounter) Add(delta float64) {
	if delta < 0 {
		return
	}
	l.parent.adapter.RecordCounter(l.parent.name, l.labels, delta)
}

func (l *labeledCounter) WithLabels(labels Labels) Counter {
	return &labeledCounter{
		parent: l.parent,
		labels: MergeLabels(l.labels, labels),
	}
}
