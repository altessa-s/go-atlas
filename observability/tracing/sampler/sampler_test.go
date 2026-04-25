// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSamplingDecision_String(t *testing.T) {
	tests := []struct {
		d    SamplingDecision
		want string
	}{
		{Drop, "Drop"},
		{RecordOnly, "RecordOnly"},
		{RecordAndSample, "RecordAndSample"},
		{SamplingDecision(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.d.String())
		})
	}
}

func TestSamplingDecision_IsSampled(t *testing.T) {
	tests := []struct {
		d    SamplingDecision
		want bool
	}{
		{Drop, false},
		{RecordOnly, false},
		{RecordAndSample, true},
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			require.Equal(t, tt.want, tt.d.IsSampled())
		})
	}
}

func TestSamplingDecision_IsRecording(t *testing.T) {
	tests := []struct {
		d    SamplingDecision
		want bool
	}{
		{Drop, false},
		{RecordOnly, true},
		{RecordAndSample, true},
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			require.Equal(t, tt.want, tt.d.IsRecording())
		})
	}
}

func TestSpanContext_IsValid(t *testing.T) {
	tests := []struct {
		name string
		sc   *SpanContext
		want bool
	}{
		{"nil", nil, false},
		{"zero", &SpanContext{}, false},
		{"valid", &SpanContext{TraceID: [16]byte{1}, SpanID: [8]byte{1}}, true},
		{"zero trace", &SpanContext{SpanID: [8]byte{1}}, false},
		{"zero span", &SpanContext{TraceID: [16]byte{1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.sc.IsValid())
		})
	}
}
