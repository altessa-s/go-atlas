// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

const (
	// DefaultIgnoreFieldsCapacity is the initial map capacity for the ignore-fields set.
	DefaultIgnoreFieldsCapacity = 8

	// DefaultFieldMappingsCapacity is the initial map capacity for field-name mappings.
	DefaultFieldMappingsCapacity = 8

	// DefaultEmbeddedStructInitialization controls whether embedded struct pointers
	// are always initialized (true) or only when they contain non-zero values (false).
	DefaultEmbeddedStructInitialization = false

	// DefaultIgnoreZeroValues controls whether zero-valued source fields are
	// skipped during conversion.
	DefaultIgnoreZeroValues = false

	// DefaultIgnoreNilValues controls whether nil source fields (pointers, slices,
	// maps) are skipped during conversion.
	DefaultIgnoreNilValues = false

	// DefaultHandleEmbeddedStructs controls whether embedded (anonymous) struct
	// fields are recursively expanded during conversion.
	DefaultHandleEmbeddedStructs = false

	// DefaultSparseMerge controls whether sparse-merge (partial-update)
	// semantics are applied: nil sources skipped, present-but-empty nested
	// structs clear the destination field.
	DefaultSparseMerge = false
)

type options struct {
	codecsSet                      *convcodec.Set
	ignoreZeroValues               bool
	ignoreNilValues                bool
	sparseMerge                    bool
	ignoreFields                   map[string]struct{}
	fieldMappings                  map[string]string
	handleEmbeddedStructs          bool
	alwaysInitializeEmbeddedStruct bool
	overflowCheck                  bool
}

func defaultOptions() *options {
	return &options{
		codecsSet:                      convcodec.NewCodecsSet(),
		ignoreFields:                   make(map[string]struct{}, DefaultIgnoreFieldsCapacity),
		fieldMappings:                  make(map[string]string, DefaultFieldMappingsCapacity),
		ignoreZeroValues:               DefaultIgnoreZeroValues,
		ignoreNilValues:                DefaultIgnoreNilValues,
		sparseMerge:                    DefaultSparseMerge,
		handleEmbeddedStructs:          DefaultHandleEmbeddedStructs,
		alwaysInitializeEmbeddedStruct: DefaultEmbeddedStructInitialization,
	}
}

func (o *options) apply(opts ...Option) *options {
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Option is a functional option for Converter configuration.
// Options allow customizing the behavior of struct conversion operations
// including field filtering, value handling, and codec registration.
type Option func(o *options)

// WithCodecs adds custom codecs to the Converter for specialized type conversions.
// Codecs are executed in the order they are provided and can transform values
// that don't have direct type compatibility. Multiple codecs can be chained
// together to create complex conversion pipelines.
func WithCodecs(codec ...convcodec.Codec) Option {
	return func(o *options) {
		o.codecsSet.Add(codec...)
	}
}

// WithIgnoreZeroValues configures the converter to skip zero values during conversion.
// When enabled, fields with zero values (empty strings, 0 for numbers, nil pointers, etc.)
// in the source struct will not be copied to the destination struct.
// This option is mutually exclusive with WithIgnoreNilValues and WithSparseMerge.
func WithIgnoreZeroValues() Option {
	return func(o *options) {
		o.ignoreZeroValues = true
		o.ignoreNilValues = DefaultIgnoreNilValues
		o.sparseMerge = DefaultSparseMerge
	}
}

// WithIgnoreNilValues configures the converter to skip nil values during conversion.
// When enabled, fields with nil values (nil pointers, nil slices, nil maps)
// in the source struct will not be copied to the destination struct.
// This option is mutually exclusive with WithIgnoreZeroValues and WithSparseMerge.
func WithIgnoreNilValues() Option {
	return func(o *options) {
		o.ignoreNilValues = true
		o.ignoreZeroValues = DefaultIgnoreZeroValues
		o.sparseMerge = DefaultSparseMerge
	}
}

// WithIgnoreFields configures the converter to ignore specific fields by name.
// Field names are case-insensitive and are converted to lowercase internally
// for consistent matching. This is useful for excluding sensitive fields
// like passwords or internal system fields from conversion.
func WithIgnoreFields(field ...string) Option {
	return func(o *options) {
		if len(field) == 0 {
			o.ignoreFields = make(map[string]struct{})
			return
		}

		o.ignoreFields = make(map[string]struct{}, max(len(field), DefaultIgnoreFieldsCapacity))
		for i := range len(field) {
			o.ignoreFields[corestrings.InternLowerString(field[i])] = struct{}{}
		}
	}
}

// WithFieldMappings configures custom field name mappings for conversion.
// This allows mapping source struct fields to destination struct fields with different names.
// Both keys (source field names) and values (destination field names) are converted to
// lowercase internally for case-insensitive matching, ensuring consistent behavior.
//
// The mappings parameter is a map where:
// - Key: source field name (will be converted to lowercase)
// - Value: destination field name (will be converted to lowercase)
//
// This is particularly useful when converting between structs with different naming
// conventions (e.g., snake_case to camelCase) or when field names don't match exactly.
func WithFieldMappings(mappings map[string]string) Option {
	return func(o *options) {
		if len(mappings) == 0 {
			o.fieldMappings = make(map[string]string)
			return
		}

		o.fieldMappings = make(map[string]string, max(len(mappings), DefaultFieldMappingsCapacity))
		for srcField, dstField := range mappings {
			o.fieldMappings[corestrings.InternLowerString(srcField)] = corestrings.InternLowerString(dstField)
		}
	}
}

// WithOverflowCheck enables runtime overflow detection for narrowing numeric conversions.
// When enabled, conversions that would silently truncate data (e.g. int64 → int8)
// panic with an OverflowError instead.
func WithOverflowCheck() Option {
	return func(o *options) {
		o.overflowCheck = true
	}
}

// WithHandleEmbeddedStructs enables processing of embedded (anonymous) structs during conversion.
// When enabled, the converter will recursively process embedded struct fields,
// allowing flattening of nested structures or proper handling of composition patterns.
//
// The initializeEmbeddedStruct parameter controls initialization behavior:
// - true: Always initialize embedded struct pointers, even if they would be empty
// - false: Only initialize embedded struct pointers if they contain non-zero values
func WithHandleEmbeddedStructs(initializeEmbeddedStruct bool) Option {
	return func(o *options) {
		o.handleEmbeddedStructs = true
		o.alwaysInitializeEmbeddedStruct = initializeEmbeddedStruct
	}
}

// WithSparseMerge configures the converter for sparse-merge (partial-update)
// semantics, applying a sparse source onto an existing destination. For each
// source field:
//   - nil pointer/slice/map -> skipped (destination left unchanged)
//   - non-nil scalar pointer (including a pointer to the zero value) -> written
//     to the destination, so an explicit empty value clears it
//   - non-nil nested struct pointer with at least one set field -> merged
//     recursively
//   - non-nil nested struct pointer with no set fields -> destination field
//     zeroed, clearing the whole nested object
//
// The semantics apply at every nesting depth. Nil sources are skipped just like
// WithIgnoreNilValues, so this option is mutually exclusive with
// WithIgnoreZeroValues and supersedes WithIgnoreNilValues.
//
// The source must express optionality through pointers:
//   - A non-pointer scalar field is always "present" and is copied as-is,
//     including its zero value — it overwrites the destination. Use pointer
//     fields for every optional scalar.
//   - A non-pointer nested struct field is likewise always "present": it is
//     merged field-by-field and never triggers the clear-on-empty rule, so an
//     untouched zero struct merges nothing instead of wiping the destination.
//
// Structs without exported fields (time.Time and similar opaque types) cannot
// be merged field-by-field and are assigned to the destination wholesale.
func WithSparseMerge() Option {
	return func(o *options) {
		o.sparseMerge = true
		o.ignoreNilValues = true
		o.ignoreZeroValues = DefaultIgnoreZeroValues
	}
}
