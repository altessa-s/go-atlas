// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pbwrap

import (
	"reflect"
	"strings"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

const (
	// WrapperPrefix is the reflect.Type.String() prefix shared by all protobuf
	// wrapper types (e.g. "wrapperspb.StringValue").
	WrapperPrefix = "wrapperspb."
	// BytesValue is excluded from conversion because its []byte nature requires
	// special handling that the generic wrapper codec does not support.
	BytesValue = "wrapperspb.BytesValue"
	// ValueFieldName is the standard field name inside every protobuf wrapper
	// struct that holds the primitive payload.
	ValueFieldName = "Value"

	// Protobuf wrapper type name constants used for documentation and testing.
	StringValue = "wrapperspb.StringValue"
	Int32Value  = "wrapperspb.Int32Value"
	Int64Value  = "wrapperspb.Int64Value"
	UInt32Value = "wrapperspb.UInt32Value"
	UInt64Value = "wrapperspb.UInt64Value"
	BoolValue   = "wrapperspb.BoolValue"
	FloatValue  = "wrapperspb.FloatValue"
	DoubleValue = "wrapperspb.DoubleValue"
)

// Registry performs protobuf wrapper conversions.
// It is intentionally stateless; conversion is done via type inspection.
type Registry struct{}

// options holds configuration options for protobuf wrapper conversion behavior.
// These options control when conversions should be skipped based on zero values.
type options struct {
	ignoreZeroValues   bool      // Skip conversion if source primitive value is zero
	ignoreZeroWrappers bool      // Skip conversion if unwrapped protobuf wrapper value is zero
	registry           *Registry // Internal registry for conversions
}

// Option defines a functional option for configuring protobuf wrapper conversion behavior.
// It follows the functional options pattern to provide flexible configuration.
type Option func(o *options)

// WithIgnoreZeroValues returns an option that skips conversion when the source primitive value is zero.
// This is useful to avoid creating protobuf wrapper objects for zero values, which can help reduce
// unnecessary allocations and maintain cleaner data structures.
func WithIgnoreZeroValues() Option {
	return func(o *options) {
		o.ignoreZeroValues = true
	}
}

// WithIgnoreZeroWrappers returns an option that skips conversion when the protobuf wrapper contains a zero value.
// This prevents unwrapping protobuf wrappers that contain zero values, maintaining the distinction between
// nil wrappers and wrappers containing zero values.
func WithIgnoreZeroWrappers() Option {
	return func(o *options) {
		o.ignoreZeroWrappers = true
	}
}

// NewRegistry creates a new protobuf conversion registry.
// The registry is pre-populated with all standard wrapper type conversions.
func NewRegistry() *Registry {
	return &Registry{}
}

// TryConvert attempts protobuf wrapper conversion.
// Returns true if conversion was handled (including skipped due to options), false otherwise.
func (r *Registry) TryConvert(src, dst reflect.Value, opts *options) bool {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	srcValue := reflect.Indirect(src)

	// Check if this is a protobuf wrapper conversion scenario
	if (!reflectutils.IsPrimitive(srcType.Kind()) || !isWrapper(dstType)) &&
		(!reflectutils.IsPrimitive(dstType.Kind()) || !isWrapper(srcType)) {
		return false
	}

	// Check for zero value options
	if opts.ignoreZeroValues && srcValue.IsZero() {
		return true
	}

	// Unwrap protobuf wrapper to primitive type
	if isWrapper(srcType) {
		unwrappedValue := unwrapWrapper(srcValue)
		if !unwrappedValue.IsValid() {
			return false
		}

		// Check for zero wrapper option BEFORE creating dst
		// Return true to indicate we "handled" it (by ignoring it)
		if opts.ignoreZeroWrappers && unwrappedValue.IsZero() {
			return true
		}

		// Only create dst if we're not ignoring this value
		reflectutils.MakeDst(&dst, dstType)
		dst.Set(unwrappedValue)
		return true
	}

	// Wrap primitive type to protobuf wrapper
	if isWrapper(dstType) {
		// Check for zero value option BEFORE creating dst
		// Return true to indicate we "handled" it (by ignoring it)
		if opts.ignoreZeroValues && srcValue.IsZero() {
			return true
		}

		// Directly handle wrapping here instead of using converter
		reflectutils.MakeDst(&dst, dstType)
		dst.FieldByName(ValueFieldName).Set(srcValue)
		return true
	}

	return false
}

// New creates a Codec for bidirectional conversion between protobuf wrapper types
// and primitive Go types. Supports StringValue, Int32Value, Int64Value, UInt32Value, UInt64Value,
// BoolValue, FloatValue, and DoubleValue. BytesValue is excluded as it requires special handling.
//
// Example:
//
//	codec := pbwrap.New()
//	codec := pbwrap.New(pbwrap.WithIgnoreZeroValues(), pbwrap.WithIgnoreZeroWrappers())
func New(opt ...Option) convcodec.Codec {
	registry := NewRegistry()
	opts := &options{
		registry: registry,
	}
	for i := range opt {
		opt[i](opts)
	}

	return func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		// Try protobuf wrapper conversions using the registry approach
		if opts.registry.TryConvert(src, dst, opts) {
			return
		}

		next(fieldName, src, dst)
	}
}

// isWrapper reports whether the given type is a protobuf wrapper type.
// It checks for struct types that have the protobuf wrapper prefix but excludes BytesValue
// which requires different handling due to its []byte nature.
func isWrapper(t reflect.Type) bool {
	return t.Kind() == reflect.Struct &&
		strings.HasPrefix(t.String(), WrapperPrefix) &&
		t.String() != BytesValue
}

// unwrapWrapper extracts the primitive value from a protobuf wrapper.
// It accesses the "Value" field of the wrapper struct and returns its reflect.Value.
func unwrapWrapper(v reflect.Value) reflect.Value {
	return reflect.Indirect(v).FieldByName(ValueFieldName)
}
