// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import "github.com/altessa-s/go-atlas/observability/metrics/adapters"

// histogram is the default implementation of Histogram.
type histogram struct {
	name       string
	labelNames []string
	buckets    []float64
	adapter    adapters.Adapter
}

func newHistogram(name string, labelNames []string, buckets []float64, adapter adapters.Adapter) *histogram {
	return &histogram{
		name:       name,
		labelNames: labelNames,
		buckets:    CopyBuckets(buckets),
		adapter:    adapter,
	}
}

func (h *histogram) Observe(value float64) {
	h.adapter.RecordHistogram(h.name, nil, value)
}

func (h *histogram) WithLabels(labels Labels) Histogram {
	return &labeledHistogram{
		parent: h,
		labels: labels,
	}
}

// labeledHistogram is a histogram with labels applied.
type labeledHistogram struct {
	parent *histogram
	labels Labels
}

func (l *labeledHistogram) Observe(value float64) {
	l.parent.adapter.RecordHistogram(l.parent.name, l.labels, value)
}

func (l *labeledHistogram) WithLabels(labels Labels) Histogram {
	return &labeledHistogram{
		parent: l.parent,
		labels: MergeLabels(l.labels, labels),
	}
}
