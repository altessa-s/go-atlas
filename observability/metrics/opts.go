// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

// MetricOpts contains options for creating basic metrics ([Counter], [Gauge]).
// Name is required; Help and LabelNames are optional.
type MetricOpts struct {
	// Name is the metric name (e.g., "requests_total", "active_connections").
	// Required. Must be non-empty.
	Name string

	// Help is a human-readable description of the metric.
	// Optional but recommended.
	Help string

	// LabelNames are the names of labels that can be attached to this metric.
	// Label values are provided via WithLabels method.
	LabelNames []string
}

// HistogramOpts contains options for creating histogram-based metrics
// ([Histogram], [Timer]).
type HistogramOpts struct {
	MetricOpts

	// Buckets defines the bucket boundaries for the histogram.
	// Must be in strictly increasing order.
	// If nil or empty, default buckets are used.
	Buckets []float64
}

// Validate validates the MetricOpts.
// Returns [ErrEmptyMetricName] or [ErrEmptyLabelName] on failure.
func (o MetricOpts) Validate() error {
	if o.Name == "" {
		return ErrEmptyMetricName
	}
	return ValidateLabelNames(o.LabelNames)
}

// Validate validates the HistogramOpts.
func (o HistogramOpts) Validate() error {
	if err := o.MetricOpts.Validate(); err != nil {
		return err
	}
	if len(o.Buckets) > 0 {
		return ValidateBuckets(o.Buckets, o.Name)
	}
	return nil
}
