// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLabelNames(t *testing.T) {
	tests := []struct {
		name    string
		labels  []string
		wantErr error
	}{
		{"nil labels", nil, nil},
		{"empty slice", []string{}, nil},
		{"valid labels", []string{"method", "path"}, nil},
		{"empty label name", []string{"method", ""}, ErrEmptyLabelName},
		{"first empty", []string{""}, ErrEmptyLabelName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabelNames(tt.labels)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   Labels
		expected []string
		wantErr  error
	}{
		{"matching", Labels{"a": "1", "b": "2"}, []string{"a", "b"}, nil},
		{"count mismatch", Labels{"a": "1"}, []string{"a", "b"}, ErrLabelCountMismatch},
		{"missing label", Labels{"a": "1", "c": "3"}, []string{"a", "b"}, ErrMissingLabel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabels(tt.labels, tt.expected)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name string
		l    Labels
		want []string
	}{
		{"nil", nil, nil},
		{"empty", Labels{}, nil},
		{"sorted", Labels{"b": "2", "a": "1", "c": "3"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SortedKeys(tt.l)
			require.True(t, slices.Equal(got, tt.want), "SortedKeys() = %v, want %v", got, tt.want)
		})
	}
}

func TestSortedValues(t *testing.T) {
	got := SortedValues(Labels{"b": "2", "a": "1"})
	want := []string{"1", "2"}
	require.True(t, slices.Equal(got, want), "SortedValues() = %v, want %v", got, want)
	require.Nil(t, SortedValues(nil))
}

func TestSortedLabelValues(t *testing.T) {
	labels := Labels{"method": "GET", "status": "200"}
	got := SortedLabelValues(labels, []string{"status", "method"})
	want := []string{"200", "GET"}
	require.True(t, slices.Equal(got, want), "SortedLabelValues() = %v, want %v", got, want)
}

func TestSortedLabelValues_MissingKey(t *testing.T) {
	labels := Labels{"method": "GET"}
	got := SortedLabelValues(labels, []string{"method", "missing"})
	want := []string{"GET", ""}
	require.True(t, slices.Equal(got, want), "SortedLabelValues() = %v, want %v", got, want)
}

func TestLabelsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Labels
		want bool
	}{
		{"both nil", nil, nil, true},
		{"equal", Labels{"a": "1"}, Labels{"a": "1"}, true},
		{"different values", Labels{"a": "1"}, Labels{"a": "2"}, false},
		{"different keys", Labels{"a": "1"}, Labels{"b": "1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, LabelsEqual(tt.a, tt.b))
		})
	}
}

func TestCloneLabels(t *testing.T) {
	orig := Labels{"a": "1", "b": "2"}
	clone := CloneLabels(orig)
	clone["c"] = "3"
	require.NotContains(t, orig, "c", "CloneLabels should create independent copy")
}

func TestMergeLabels(t *testing.T) {
	base := Labels{"a": "1", "b": "2"}
	other := Labels{"b": "override", "c": "3"}
	result := MergeLabels(base, other)
	require.Equal(t, "override", result["b"])
	require.Equal(t, "1", result["a"])
}

func TestNormalizeLabelNames(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"sorted", []string{"c", "a", "b"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLabelNames(tt.names)
			require.True(t, slices.Equal(got, tt.want), "NormalizeLabelNames() = %v, want %v", got, tt.want)
		})
	}
}

func TestLabelBuilder(t *testing.T) {
	labels := NewLabelBuilder().
		Add("method", "GET").
		Add("path", "/api").
		AddIf(true, "status", "200").
		AddIf(false, "skip", "value").
		AddNonEmpty("host", "localhost").
		AddNonEmpty("empty", "").
		Build()

	require.Equal(t, "GET", labels["method"])
	require.NotContains(t, labels, "skip", "AddIf(false) should not add")
	require.NotContains(t, labels, "empty", "AddNonEmpty with empty value should not add")
	require.Equal(t, "localhost", labels["host"])
}

func TestLabelBuilder_Merge(t *testing.T) {
	labels := NewLabelBuilder().
		Add("a", "1").
		Merge(Labels{"b": "2", "a": "override"}).
		Build()

	require.Equal(t, "override", labels["a"], "Merge should override")
	require.Equal(t, "2", labels["b"], "Merge should add new")
}

func TestLabelBuilder_Reset(t *testing.T) {
	b := NewLabelBuilder().Add("a", "1")
	b.Reset()
	labels := b.Build()
	require.Empty(t, labels, "Reset should clear")
}

func TestLabelBuilder_BuildClone(t *testing.T) {
	b := NewLabelBuilder().Add("a", "1")
	clone := b.BuildClone()
	clone["b"] = "2"
	original := b.Build()
	require.NotContains(t, original, "b", "BuildClone should return independent copy")
}

func TestLabelsPool(t *testing.T) {
	l := GetLabels()
	require.NotNil(t, l)
	(*l)["key"] = "val"
	PutLabels(l)

	l2 := GetLabelsWithCapacity(16)
	require.NotNil(t, l2)
	PutLabels(l2)
}

func TestSortedLabelsIter(t *testing.T) {
	labels := Labels{"b": "2", "a": "1"}
	var keys []string
	for k := range SortedLabelsIter(labels) {
		keys = append(keys, k)
	}
	require.True(t, slices.Equal(keys, []string{"a", "b"}), "SortedLabelsIter keys = %v", keys)
}
