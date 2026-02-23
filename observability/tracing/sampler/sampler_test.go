// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import "testing"

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
			if got := tt.d.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
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
			if got := tt.d.IsSampled(); got != tt.want {
				t.Errorf("IsSampled() = %v, want %v", got, tt.want)
			}
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
			if got := tt.d.IsRecording(); got != tt.want {
				t.Errorf("IsRecording() = %v, want %v", got, tt.want)
			}
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
			if got := tt.sc.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
