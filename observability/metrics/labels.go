// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"iter"
	"maps"
	"slices"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
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

// MergeLabels merges labels, with values from other overriding values in base.
// Uses coremaps.Merge from the core package.
func MergeLabels(base, other Labels) Labels {
	return coremaps.Merge(other, base)
}

// SortedKeys returns a sorted slice of keys.
// Uses slices.Sorted from Go 1.23+.
func SortedKeys(l Labels) []string {
	if len(l) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(l))
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
	if slices.Contains(labelNames, "") {
		return ErrEmptyLabelName
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

// NormalizeLabelNames returns a sorted copy of label names.
func NormalizeLabelNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(names))
}

type labelKV struct {
	k, v string
}

// labelSnapshot holds an immutable copy of a caller-supplied Labels map so
// later caller mutations never affect bound handles. When a full map is not
// needed (pre-bound handles) and at most two labels are present, pairs are
// stored inline to avoid a map allocation.
type labelSnapshot struct {
	m       Labels
	inline  [2]labelKV
	nInline uint8
}

func snapshotLabels(labels Labels, needMap bool) labelSnapshot {
	if len(labels) == 0 {
		return labelSnapshot{}
	}
	if !needMap && len(labels) <= len(labelSnapshot{}.inline) {
		var s labelSnapshot
		for k, v := range labels {
			s.inline[s.nInline] = labelKV{k: k, v: v}
			s.nInline++
		}
		return s
	}
	return labelSnapshot{m: maps.Clone(labels)}
}

func (s *labelSnapshot) merge(other Labels) Labels {
	if s.nInline == 0 {
		return MergeLabels(s.m, other)
	}
	out := make(Labels, int(s.nInline)+len(other))
	for i := range s.nInline {
		kv := s.inline[i]
		out[kv.k] = kv.v
	}
	maps.Copy(out, other)
	return out
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
	maps.Copy(b.labels, other)
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
	return maps.Clone(b.labels)
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
