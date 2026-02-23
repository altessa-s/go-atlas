// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"testing"
)

func TestValidateBuckets(t *testing.T) {
	tests := []struct {
		name    string
		buckets []float64
		wantErr bool
	}{
		{"valid_increasing", []float64{0.1, 0.5, 1.0, 5.0}, false},
		{"single", []float64{1.0}, false},
		{"empty", []float64{}, true},
		{"not_increasing", []float64{1.0, 0.5, 2.0}, true},
		{"duplicates", []float64{1.0, 1.0, 2.0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuckets(tt.buckets, "test")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateBuckets() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMustValidateBuckets_Valid(t *testing.T) {
	MustValidateBuckets([]float64{0.1, 0.5, 1.0}, "test")
}

func TestMustValidateBuckets_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustValidateBuckets([]float64{}, "test")
}

func TestCopyBuckets(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := CopyBuckets(nil); got != nil {
			t.Fatalf("CopyBuckets(nil) = %v, want nil", got)
		}
	})

	t.Run("copy_is_independent", func(t *testing.T) {
		orig := []float64{1.0, 2.0, 3.0}
		cp := CopyBuckets(orig)
		cp[0] = 999
		if orig[0] == 999 {
			t.Fatal("CopyBuckets did not create independent copy")
		}
	})
}

func TestDefaultBuckets(t *testing.T) {
	if err := ValidateBuckets(DefaultDurationBuckets, "duration"); err != nil {
		t.Fatalf("DefaultDurationBuckets invalid: %v", err)
	}
	if err := ValidateBuckets(DefaultSizeBuckets, "size"); err != nil {
		t.Fatalf("DefaultSizeBuckets invalid: %v", err)
	}
}

func TestBuildMetricName(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		subsystem string
		suffix    string
		want      string
	}{
		{"all_parts", "myapp", "http", "requests_total", "myapp_http_requests_total"},
		{"no_namespace", "", "grpc", "requests_total", "grpc_requests_total"},
		{"no_subsystem", "myapp", "", "requests_total", "myapp_requests_total"},
		{"suffix_only", "", "", "requests_total", "requests_total"},
		{"all_empty", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildMetricName(tt.namespace, tt.subsystem, tt.suffix); got != tt.want {
				t.Fatalf("BuildMetricName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMetricNameWithDefault(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		subsystem     string
		suffix        string
		defaultPrefix string
		want          string
	}{
		{"with_namespace", "myapp", "http", "requests_total", "http_", "myapp_http_requests_total"},
		{"fallback_to_default", "", "", "requests_total", "http_", "http_requests_total"},
		{"all_empty_no_default", "", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMetricNameWithDefault(tt.namespace, tt.subsystem, tt.suffix, tt.defaultPrefix)
			if got != tt.want {
				t.Fatalf("BuildMetricNameWithDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetMessageSize(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		size, ok := GetMessageSize(nil)
		if ok || size != 0 {
			t.Fatalf("GetMessageSize(nil) = (%d, %v), want (0, false)", size, ok)
		}
	})

	t.Run("ProtoSize", func(t *testing.T) {
		msg := &mockProtoSizer{size: 42}
		size, ok := GetMessageSize(msg)
		if !ok || size != 42 {
			t.Fatalf("GetMessageSize(ProtoSize) = (%d, %v), want (42, true)", size, ok)
		}
	})

	t.Run("Size", func(t *testing.T) {
		msg := &mockSizer{size: 99}
		size, ok := GetMessageSize(msg)
		if !ok || size != 99 {
			t.Fatalf("GetMessageSize(Size) = (%d, %v), want (99, true)", size, ok)
		}
	})

	t.Run("XXX_Size", func(t *testing.T) {
		msg := &mockLegacySizer{size: 55}
		size, ok := GetMessageSize(msg)
		if !ok || size != 55 {
			t.Fatalf("GetMessageSize(XXX_Size) = (%d, %v), want (55, true)", size, ok)
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		size, ok := GetMessageSize("just a string")
		if ok || size != 0 {
			t.Fatalf("GetMessageSize(string) = (%d, %v), want (0, false)", size, ok)
		}
	})
}

func TestLabels_NonEmpty(t *testing.T) {
	labels := []struct {
		name  string
		value string
	}{
		{"MethodLabel", MethodLabel},
		{"StatusLabel", StatusLabel},
		{"PathLabel", PathLabel},
		{"DirectionLabel", DirectionLabel},
		{"DirectionSent", DirectionSent},
		{"DirectionReceived", DirectionReceived},
	}

	for _, tt := range labels {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Fatalf("%s is empty", tt.name)
			}
		})
	}
}

type mockProtoSizer struct{ size int }

func (m *mockProtoSizer) ProtoSize() int { return m.size }

type mockSizer struct{ size int }

func (m *mockSizer) Size() int { return m.size }

type mockLegacySizer struct{ size int }

func (m *mockLegacySizer) XXX_Size() int { return m.size }
