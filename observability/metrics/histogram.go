// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync/atomic"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
)

// histogram is the default implementation of Histogram.
type histogram struct {
	name     string
	adapter  adapters.Adapter
	shutdown *atomic.Bool
	bound    adapters.BoundHistogram // unlabeled child, bound at registration when supported
}

// bindUnlabeled resolves the unlabeled child once. The collector calls it
// after registration and before publishing the handle.
func (h *histogram) bindUnlabeled() {
	if b, ok := h.adapter.(adapters.Binder); ok {
		if bh, ok := b.BindHistogram(h.name, nil); ok {
			h.bound = bh
		}
	}
}

func newHistogram(name string, adapter adapters.Adapter, shutdown *atomic.Bool) *histogram {
	if shutdown == nil {
		shutdown = &activeLifecycle
	}
	return &histogram{
		name:     name,
		adapter:  adapter,
		shutdown: shutdown,
	}
}

func (h *histogram) Observe(value float64) {
	if h.shutdown.Load() {
		return
	}
	if h.bound != nil {
		h.bound.Observe(value)
		return
	}
	h.adapter.RecordHistogram(h.name, nil, value)
}

func (h *histogram) WithLabels(labels Labels) Histogram {
	if h.shutdown.Load() {
		return noopHistogram{}
	}
	return newLabeledHistogram(h, labels)
}

// newLabeledHistogram builds a labeled view with an immutable copy of labels
// and, when the adapter supports [adapters.Binder], resolves the concrete child
// once so Observe is a direct update with no per-observation lookup.
func newLabeledHistogram(parent *histogram, labels Labels) *labeledHistogram {
	var bound adapters.BoundHistogram
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindHistogram(parent.name, labels); ok {
			bound = h
		}
	}
	return &labeledHistogram{
		parent:   parent,
		shutdown: parent.shutdown,
		labels:   snapshotLabels(labels, bound == nil),
		bound:    bound,
	}
}

// labeledHistogram is a histogram with labels applied.
type labeledHistogram struct {
	parent   *histogram
	shutdown *atomic.Bool
	labels   labelSnapshot
	bound    adapters.BoundHistogram // non-nil when the adapter pre-resolved the child
}

func (l *labeledHistogram) Observe(value float64) {
	if l.shutdown.Load() {
		return
	}
	if l.bound != nil {
		l.bound.Observe(value)
		return
	}
	l.parent.adapter.RecordHistogram(l.parent.name, l.labels.m, value)
}

func (l *labeledHistogram) WithLabels(labels Labels) Histogram {
	if l.shutdown.Load() {
		return noopHistogram{}
	}
	return newLabeledHistogram(l.parent, l.labels.merge(labels))
}
