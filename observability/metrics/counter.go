// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// counter is the default implementation of Counter, delegating every
// observation to the adapter. It keeps no shadow value: nothing reads it,
// and a local CAS-loop float add per increment would only double-account
// what the adapter already stores.
type counter struct {
	name     string
	adapter  adapters.Adapter
	shutdown *atomic.Bool
	bound    adapters.BoundCounter // unlabeled child, bound at registration when supported
}

// bindUnlabeled resolves the unlabeled child once. The collector calls it
// after registration and before publishing the handle.
func (c *counter) bindUnlabeled() {
	if b, ok := c.adapter.(adapters.Binder); ok {
		if h, ok := b.BindCounter(c.name, nil); ok {
			c.bound = h
		}
	}
}

func newCounter(name string, adapter adapters.Adapter, shutdown *atomic.Bool) *counter {
	if shutdown == nil {
		shutdown = &activeLifecycle
	}
	return &counter{
		name:     name,
		adapter:  adapter,
		shutdown: shutdown,
	}
}

func (c *counter) Inc() {
	c.Add(1)
}

func (c *counter) Add(delta float64) {
	if delta < 0 || c.shutdown.Load() {
		return // Counters can only increase and are inert after shutdown.
	}
	if c.bound != nil {
		c.bound.Add(delta)
		return
	}
	c.adapter.RecordCounter(c.name, nil, delta)
}

func (c *counter) WithLabels(labels Labels) Counter {
	if c.shutdown.Load() {
		return noopCounter{}
	}
	return newLabeledCounter(c, labels)
}

// newLabeledCounter builds a labeled view and,
// when the adapter supports [adapters.Binder], resolves the concrete child once
// so Inc/Add are direct updates with no per-observation lookup.
func newLabeledCounter(parent *counter, labels Labels) *labeledCounter {
	var bound adapters.BoundCounter
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindCounter(parent.name, labels); ok {
			bound = h
		}
	}
	return &labeledCounter{
		parent: parent,
		labels: retainLabels(labels, bound == nil),
		bound:  bound,
	}
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
	if delta < 0 || l.parent.shutdown.Load() {
		return
	}
	if l.bound != nil {
		l.bound.Add(delta)
		return
	}
	l.parent.adapter.RecordCounter(l.parent.name, l.labels, delta)
}

func (l *labeledCounter) WithLabels(labels Labels) Counter {
	if l.parent.shutdown.Load() {
		return noopCounter{}
	}
	return newLabeledCounter(l.parent, MergeLabels(l.labels, labels))
}
