// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
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
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
