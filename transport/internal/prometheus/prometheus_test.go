// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.wantErr, (err != nil))
		})
	}
}

func TestMustValidateBuckets_Valid(t *testing.T) {
	MustValidateBuckets([]float64{0.1, 0.5, 1.0}, "test")
}

func TestMustValidateBuckets_Panics(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r)
	}()
	MustValidateBuckets([]float64{}, "test")
}

func TestCopyBuckets(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := CopyBuckets(nil)
		require.Nil(t, got)
	})

	t.Run("copy_is_independent", func(t *testing.T) {
		orig := []float64{1.0, 2.0, 3.0}
		cp := CopyBuckets(orig)
		cp[0] = 999
		require.NotEqual(t, 999, orig[0])
	})
}

func TestDefaultBuckets(t *testing.T) {
	require.NoError(t, ValidateBuckets(DefaultDurationBuckets, "duration"))
	require.NoError(t, ValidateBuckets(DefaultSizeBuckets, "size"))
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
			got := BuildMetricName(tt.namespace, tt.subsystem, tt.suffix)
			require.Equal(t, tt.want, got)
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
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetMessageSize(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		size, ok := GetMessageSize(nil)
		require.False(t, ok)
		require.Equal(t, 0, size)
	})

	t.Run("ProtoSize", func(t *testing.T) {
		msg := &mockProtoSizer{size: 42}
		size, ok := GetMessageSize(msg)
		require.True(t, ok)
		require.Equal(t, 42, size)
	})

	t.Run("Size", func(t *testing.T) {
		msg := &mockSizer{size: 99}
		size, ok := GetMessageSize(msg)
		require.True(t, ok)
		require.Equal(t, 99, size)
	})

	t.Run("XXX_Size", func(t *testing.T) {
		msg := &mockLegacySizer{size: 55}
		size, ok := GetMessageSize(msg)
		require.True(t, ok)
		require.Equal(t, 55, size)
	})

	t.Run("unknown_type", func(t *testing.T) {
		size, ok := GetMessageSize("just a string")
		require.False(t, ok)
		require.Equal(t, 0, size)
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
			require.NotEqual(t, "", tt.value)
		})
	}
}

type mockProtoSizer struct{ size int }

func (m *mockProtoSizer) ProtoSize() int { return m.size }

type mockSizer struct{ size int }

func (m *mockSizer) Size() int { return m.size }

type mockLegacySizer struct{ size int }

func (m *mockLegacySizer) XXX_Size() int { return m.size }
