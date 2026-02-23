// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"slices"
	"testing"
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
			if err != tt.wantErr {
				t.Errorf("ValidateLabelNames() = %v, want %v", err, tt.wantErr)
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
			if err != tt.wantErr {
				t.Errorf("ValidateLabels() = %v, want %v", err, tt.wantErr)
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
			if !slices.Equal(got, tt.want) {
				t.Errorf("SortedKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortedValues(t *testing.T) {
	got := SortedValues(Labels{"b": "2", "a": "1"})
	want := []string{"1", "2"}
	if !slices.Equal(got, want) {
		t.Errorf("SortedValues() = %v, want %v", got, want)
	}
	if SortedValues(nil) != nil {
		t.Error("SortedValues(nil) should return nil")
	}
}

func TestSortedLabelValues(t *testing.T) {
	labels := Labels{"method": "GET", "status": "200"}
	got := SortedLabelValues(labels, []string{"status", "method"})
	want := []string{"200", "GET"}
	if !slices.Equal(got, want) {
		t.Errorf("SortedLabelValues() = %v, want %v", got, want)
	}
}

func TestSortedLabelValues_MissingKey(t *testing.T) {
	labels := Labels{"method": "GET"}
	got := SortedLabelValues(labels, []string{"method", "missing"})
	want := []string{"GET", ""}
	if !slices.Equal(got, want) {
		t.Errorf("SortedLabelValues() = %v, want %v", got, want)
	}
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
			if got := LabelsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("LabelsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCloneLabels(t *testing.T) {
	orig := Labels{"a": "1", "b": "2"}
	clone := CloneLabels(orig)
	clone["c"] = "3"
	if _, ok := orig["c"]; ok {
		t.Error("CloneLabels should create independent copy")
	}
}

func TestMergeLabels(t *testing.T) {
	base := Labels{"a": "1", "b": "2"}
	other := Labels{"b": "override", "c": "3"}
	result := MergeLabels(base, other)
	if result["b"] != "override" {
		t.Errorf("expected override, got %s", result["b"])
	}
	if result["a"] != "1" {
		t.Errorf("expected 1, got %s", result["a"])
	}
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
			if !slices.Equal(got, tt.want) {
				t.Errorf("NormalizeLabelNames() = %v, want %v", got, tt.want)
			}
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

	if labels["method"] != "GET" {
		t.Errorf("expected GET, got %s", labels["method"])
	}
	if _, ok := labels["skip"]; ok {
		t.Error("AddIf(false) should not add")
	}
	if _, ok := labels["empty"]; ok {
		t.Error("AddNonEmpty with empty value should not add")
	}
	if labels["host"] != "localhost" {
		t.Errorf("expected localhost, got %s", labels["host"])
	}
}

func TestLabelBuilder_Merge(t *testing.T) {
	labels := NewLabelBuilder().
		Add("a", "1").
		Merge(Labels{"b": "2", "a": "override"}).
		Build()

	if labels["a"] != "override" {
		t.Errorf("Merge should override: got %s", labels["a"])
	}
	if labels["b"] != "2" {
		t.Errorf("Merge should add new: got %s", labels["b"])
	}
}

func TestLabelBuilder_Reset(t *testing.T) {
	b := NewLabelBuilder().Add("a", "1")
	b.Reset()
	labels := b.Build()
	if len(labels) != 0 {
		t.Errorf("Reset should clear: len=%d", len(labels))
	}
}

func TestLabelBuilder_BuildClone(t *testing.T) {
	b := NewLabelBuilder().Add("a", "1")
	clone := b.BuildClone()
	clone["b"] = "2"
	original := b.Build()
	if _, ok := original["b"]; ok {
		t.Error("BuildClone should return independent copy")
	}
}

func TestLabelsPool(t *testing.T) {
	l := GetLabels()
	if l == nil {
		t.Fatal("GetLabels() returned nil")
	}
	(*l)["key"] = "val"
	PutLabels(l)

	l2 := GetLabelsWithCapacity(16)
	if l2 == nil {
		t.Fatal("GetLabelsWithCapacity() returned nil")
	}
	PutLabels(l2)
}

func TestSortedLabelsIter(t *testing.T) {
	labels := Labels{"b": "2", "a": "1"}
	var keys []string
	for k := range SortedLabelsIter(labels) {
		keys = append(keys, k)
	}
	if !slices.Equal(keys, []string{"a", "b"}) {
		t.Errorf("SortedLabelsIter keys = %v", keys)
	}
}
