// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"errors"
	"testing"
)

func TestMetricOpts_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    MetricOpts
		wantErr error
	}{
		{"valid", MetricOpts{Name: "requests"}, nil},
		{"with labels", MetricOpts{Name: "requests", LabelNames: []string{"method"}}, nil},
		{"empty name", MetricOpts{}, ErrEmptyMetricName},
		{"empty label", MetricOpts{Name: "requests", LabelNames: []string{""}}, ErrEmptyLabelName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestHistogramOpts_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    HistogramOpts
		wantErr bool
	}{
		{"valid no buckets", HistogramOpts{MetricOpts: MetricOpts{Name: "dur"}}, false},
		{"valid with buckets", HistogramOpts{MetricOpts: MetricOpts{Name: "dur"}, Buckets: []float64{0.1, 0.5, 1.0}}, false},
		{"empty name", HistogramOpts{}, true},
		{"invalid buckets", HistogramOpts{MetricOpts: MetricOpts{Name: "dur"}, Buckets: []float64{1.0, 0.5}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
