// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// timer is the default implementation of Timer.
type timer struct {
	*histogram
}

func newTimer(name string, adapter adapters.Adapter, shutdown *atomic.Bool) *timer {
	return &timer{
		histogram: newHistogram(name, adapter, shutdown),
	}
}

func (t *timer) Start() func() {
	if t.shutdown.Load() {
		return func() {}
	}
	start := time.Now()
	return func() {
		t.ObserveDuration(time.Since(start))
	}
}

func (t *timer) ObserveDuration(d time.Duration) {
	t.Observe(d.Seconds())
}

func (t *timer) WithLabels(labels Labels) Timer {
	if t.shutdown.Load() {
		return noopTimer{}
	}
	return newLabeledTimer(t, labels)
}

// newLabeledTimer builds a labeled view with an immutable copy of labels and,
// when the adapter supports [adapters.Binder], resolves the concrete child once
// so ObserveDuration is a direct update with no per-observation lookup.
func newLabeledTimer(parent *timer, labels Labels) *labeledTimer {
	var bound adapters.BoundHistogram
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindHistogram(parent.name, labels); ok {
			bound = h
		}
	}
	return &labeledTimer{
		parent:   parent,
		shutdown: parent.shutdown,
		labels:   snapshotLabels(labels, bound == nil),
		bound:    bound,
	}
}

// labeledTimer is a timer with labels applied.
type labeledTimer struct {
	parent   *timer
	shutdown *atomic.Bool
	labels   labelSnapshot
	bound    adapters.BoundHistogram // non-nil when the adapter pre-resolved the child
}

func (l *labeledTimer) Start() func() {
	if l.shutdown.Load() {
		return func() {}
	}
	start := time.Now()
	return func() {
		l.ObserveDuration(time.Since(start))
	}
}

func (l *labeledTimer) ObserveDuration(d time.Duration) {
	if l.shutdown.Load() {
		return
	}
	if l.bound != nil {
		l.bound.Observe(d.Seconds())
		return
	}
	l.parent.adapter.RecordHistogram(l.parent.name, l.labels.m, d.Seconds())
}

func (l *labeledTimer) WithLabels(labels Labels) Timer {
	if l.shutdown.Load() {
		return noopTimer{}
	}
	return newLabeledTimer(l.parent, l.labels.merge(labels))
}
