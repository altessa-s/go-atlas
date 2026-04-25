// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"iter"
	"maps"
	"slices"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// Labels represents a set of label key-value pairs for metrics.
// Labels are used to add dimensions to metrics, allowing filtering and grouping.
type Labels = map[string]string

// labelsPool provides pooling for Labels maps.
var labelsPool = coremaps.NewPool[string, string](8) //nolint:mnd // Pool capacity hint

// GetLabels retrieves a Labels map from the pool.
// Use PutLabels to return it to the pool after use.
func GetLabels() *Labels {
	return labelsPool.Get()
}

// GetLabelsWithCapacity retrieves a Labels map from the pool with the given capacity.
func GetLabelsWithCapacity(capacity int) *Labels {
	return labelsPool.GetWithCapacity(capacity)
}

// PutLabels returns a Labels map to the pool for reuse.
func PutLabels(l *Labels) {
	labelsPool.Put(l)
}

// CloneLabels creates a copy of labels.
// Uses maps.Clone from the standard library (Go 1.21+).
func CloneLabels(l Labels) Labels {
	return maps.Clone(l)
}

// MergeLabels merges labels, with values from other overriding values in base.
// Uses coremaps.Merge from the core package.
func MergeLabels(base, other Labels) Labels {
	return coremaps.Merge(other, base)
}

// LabelsKeys returns an iterator over label keys.
// Uses coremaps.Keys from the core package.
func LabelsKeys(l Labels) iter.Seq[string] {
	return coremaps.Keys(l)
}

// LabelsValues returns an iterator over label values.
func LabelsValues(l Labels) iter.Seq[string] {
	return coremaps.Values(l)
}

// SortedKeys returns a sorted slice of keys.
// Uses slices.Sorted from Go 1.23+.
func SortedKeys(l Labels) []string {
	if len(l) == 0 {
		return nil
	}
	return slices.Sorted(coremaps.Keys(l))
}

// SortedValues returns values in the order of sorted keys.
func SortedValues(l Labels) []string {
	keys := SortedKeys(l)
	if keys == nil {
		return nil
	}
	values := make([]string, len(keys))
	for i, k := range keys {
		values[i] = l[k]
	}
	return values
}

// SortedLabelValues returns label values in the order of specified labelNames.
// Missing labels are returned as empty strings.
func SortedLabelValues(labels Labels, labelNames []string) []string {
	values := make([]string, len(labelNames))
	for i, name := range labelNames {
		values[i] = labels[name]
	}
	return values
}

// ValidateLabelNames validates that all label names are non-empty.
// Returns [ErrEmptyLabelName] if any name is empty.
func ValidateLabelNames(labelNames []string) error {
	for _, name := range labelNames {
		if name == "" {
			return ErrEmptyLabelName
		}
	}
	return nil
}

// ValidateLabels validates that labels contain all expected names.
// Returns [ErrLabelCountMismatch] or [ErrMissingLabel] on failure.
func ValidateLabels(labels Labels, expectedNames []string) error {
	if len(labels) != len(expectedNames) {
		return ErrLabelCountMismatch
	}

	for _, name := range expectedNames {
		if _, ok := labels[name]; !ok {
			return ErrMissingLabel
		}
	}

	return nil
}

// LabelsEqual checks equality of two Labels.
// Uses maps.Equal from the standard library.
func LabelsEqual(a, b Labels) bool {
	return maps.Equal(a, b)
}

// FilterLabels returns an iterator over filtered labels.
func FilterLabels(l Labels, predicate func(key, value string) bool) iter.Seq2[string, string] {
	return coremaps.Filter(l, predicate)
}

// NormalizeLabelNames returns a sorted copy of label names.
func NormalizeLabelNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(names))
}

// --- LabelBuilder ---

// LabelBuilder provides a fluent API for building [Labels].
// Uses pooled maps from [GetLabels] for efficiency.
// Not safe for concurrent use.
//
// Example:
//
//	labels := NewLabelBuilder().
//	    Add("method", "GET").
//	    Add("path", "/api/users").
//	    Add("status", "200").
//	    Build()
type LabelBuilder struct {
	labels Labels
}

// NewLabelBuilder creates a new LabelBuilder with a pooled map.
func NewLabelBuilder() *LabelBuilder {
	return &LabelBuilder{labels: *GetLabels()}
}

// NewLabelBuilderWithCapacity creates a new LabelBuilder with specified capacity.
func NewLabelBuilderWithCapacity(capacity int) *LabelBuilder {
	return &LabelBuilder{labels: *GetLabelsWithCapacity(capacity)}
}

// Add adds a key-value pair to the labels.
// Returns the builder for method chaining.
func (b *LabelBuilder) Add(key, value string) *LabelBuilder {
	b.labels[key] = value
	return b
}

// AddIf conditionally adds a key-value pair to the labels.
// The pair is only added if condition is true.
// Returns the builder for method chaining.
func (b *LabelBuilder) AddIf(condition bool, key, value string) *LabelBuilder {
	if condition {
		b.labels[key] = value
	}
	return b
}

// AddNonEmpty adds a key-value pair only if value is non-empty.
// Returns the builder for method chaining.
func (b *LabelBuilder) AddNonEmpty(key, value string) *LabelBuilder {
	if value != "" {
		b.labels[key] = value
	}
	return b
}

// Merge merges another Labels map into this builder.
// Values from the other map overwrite existing values.
// Returns the builder for method chaining.
func (b *LabelBuilder) Merge(other Labels) *LabelBuilder {
	for k, v := range other {
		b.labels[k] = v
	}
	return b
}

// Build returns the built Labels map.
// The builder should not be used after calling Build.
func (b *LabelBuilder) Build() Labels {
	return b.labels
}

// BuildClone returns a clone of the built Labels map.
// The builder can continue to be used after calling BuildClone.
func (b *LabelBuilder) BuildClone() Labels {
	return CloneLabels(b.labels)
}

// Reset clears the builder for reuse.
// Returns the builder for method chaining.
func (b *LabelBuilder) Reset() *LabelBuilder {
	clear(b.labels)
	return b
}

// --- Sorted Labels Iterator ---

// SortedLabelsIter returns an iterator over labels in sorted key order.
// Use for deterministic iteration when order matters.
//
// Example:
//
//	for k, v := range SortedLabelsIter(labels) {
//	    fmt.Printf("%s=%s\n", k, v)
//	}
func SortedLabelsIter(l Labels) iter.Seq2[string, string] {
	keys := SortedKeys(l)
	return func(yield func(string, string) bool) {
		for _, k := range keys {
			if !yield(k, l[k]) {
				return
			}
		}
	}
}

// LabelsIter returns an iterator over labels (unordered).
// For ordered iteration, use SortedLabelsIter.
//
// Example:
//
//	for k, v := range LabelsIter(labels) {
//	    fmt.Printf("%s=%s\n", k, v)
//	}
func LabelsIter(l Labels) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for k, v := range l {
			if !yield(k, v) {
				return
			}
		}
	}
}
