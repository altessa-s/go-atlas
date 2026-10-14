// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "github.com/altessa-s/go-atlas/observability/metrics/adapters"

// histogram is the default implementation of Histogram.
type histogram struct {
	name    string
	adapter adapters.Adapter
	bound   adapters.BoundHistogram // unlabeled child, bound at registration when supported
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

func newHistogram(name string, adapter adapters.Adapter) *histogram {
	return &histogram{
		name:    name,
		adapter: adapter,
	}
}

func (h *histogram) Observe(value float64) {
	if h.bound != nil {
		h.bound.Observe(value)
		return
	}
	h.adapter.RecordHistogram(h.name, nil, value)
}

func (h *histogram) WithLabels(labels Labels) Histogram {
	return newLabeledHistogram(h, labels)
}

// newLabeledHistogram builds a labeled view and, when the adapter supports
// [adapters.Binder], resolves the concrete child once so Observe is a direct
// update with no per-observation lookup.
func newLabeledHistogram(parent *histogram, labels Labels) *labeledHistogram {
	l := &labeledHistogram{parent: parent, labels: labels}
	if b, ok := parent.adapter.(adapters.Binder); ok {
		if h, ok := b.BindHistogram(parent.name, labels); ok {
			l.bound = h
		}
	}
	return l
}

// labeledHistogram is a histogram with labels applied.
type labeledHistogram struct {
	parent *histogram
	labels Labels
	bound  adapters.BoundHistogram // non-nil when the adapter pre-resolved the child
}

func (l *labeledHistogram) Observe(value float64) {
	if l.bound != nil {
		l.bound.Observe(value)
		return
	}
	l.parent.adapter.RecordHistogram(l.parent.name, l.labels, value)
}

func (l *labeledHistogram) WithLabels(labels Labels) Histogram {
	return newLabeledHistogram(l.parent, MergeLabels(l.labels, labels))
}
