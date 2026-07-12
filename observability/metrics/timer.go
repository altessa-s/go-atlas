// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// timer is the default implementation of Timer.
type timer struct {
	*histogram
}

func newTimer(name string, labelNames []string, buckets []float64, adapter adapters.Adapter) *timer {
	return &timer{
		histogram: newHistogram(name, labelNames, buckets, adapter),
	}
}

func (t *timer) Start() func() {
	start := time.Now()
	return func() {
		t.ObserveDuration(time.Since(start))
	}
}

func (t *timer) ObserveDuration(d time.Duration) {
	t.Observe(d.Seconds())
}

func (t *timer) WithLabels(labels Labels) Timer {
	return newLabeledTimer(t, labels)
}

// newLabeledTimer builds a labeled view and, when the adapter supports
// [adapters.Binder], resolves the concrete child once so ObserveDuration is
// a direct update with no per-observation lookup.
func newLabeledTimer(parent *timer, labels Labels) *labeledTimer {
	l := &labeledTimer{parent: parent, labels: labels}
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindHistogram(parent.name, labels); ok {
			l.bound = h
		}
	}
	return l
}

// labeledTimer is a timer with labels applied.
type labeledTimer struct {
	parent *timer
	labels Labels
	bound  adapters.BoundHistogram // non-nil when the adapter pre-resolved the child
}

func (l *labeledTimer) Start() func() {
	start := time.Now()
	return func() {
		l.ObserveDuration(time.Since(start))
	}
}

func (l *labeledTimer) ObserveDuration(d time.Duration) {
	if l.bound != nil {
		l.bound.Observe(d.Seconds())
		return
	}
	l.parent.adapter.RecordHistogram(l.parent.name, l.labels, d.Seconds())
}

func (l *labeledTimer) WithLabels(labels Labels) Timer {
	return newLabeledTimer(l.parent, MergeLabels(l.labels, labels))
}
