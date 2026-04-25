// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mapslice

import (
	"reflect"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

// optimizer provides optimized type checking for map-to-slice conversions.
type optimizer struct {
	srcKind      reflect.Kind
	dstKind      reflect.Kind
	isMapToSlice bool
}

// newOptimizer creates an optimizer for the given types.
// Returns an optimizer that caches type information for fast conversion checks.
func newOptimizer(srcType, dstType reflect.Type) *optimizer {
	srcKind := srcType.Kind()
	dstKind := dstType.Kind()
	return &optimizer{
		srcKind:      srcKind,
		dstKind:      dstKind,
		isMapToSlice: srcKind == reflect.Map && dstKind == reflect.Slice,
	}
}

// canConvert checks if the conversion is possible based on element kinds.
// Returns true if element kinds match or destination is any.
func (opt *optimizer) canConvert(srcElemKind, dstElemKind reflect.Kind) bool {
	return opt.isMapToSlice && (srcElemKind == dstElemKind || dstElemKind == reflect.Interface)
}

// Values is a Codec that converts map values to a slice.
// The map value type must match the slice element type, or slice elements must be any.
// If the destination slice is nil, a new slice is created with appropriate capacity.
// The order of values corresponds to the iteration order of map keys (not guaranteed).
//
// Example:
//
//	source := map[string]int{"a": 1, "b": 2}
//	var dst []int
//	// After conversion: dst contains [1, 2] (order may vary)
var Values convcodec.Codec = func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	// Use optimized type checking
	opt := newOptimizer(srcType, dstType)
	if !opt.isMapToSlice {
		next(fieldName, src, dst)
		return
	}

	// skip if the type of slice elements does not match the type of map values
	if !opt.canConvert(srcType.Elem().Kind(), dstType.Elem().Kind()) {
		next(fieldName, src, dst)
		return
	}

	if dst.IsNil() {
		// create a new pointer if dst is nil
		dst.Set(reflect.MakeSlice(reflect.SliceOf(dstType.Elem()), 0, len(src.MapKeys())))
	}

	keys := src.MapKeys()
	for i := range keys {
		dst.Set(reflect.Append(dst, src.MapIndex(keys[i])))
	}
}

// Keys is a Codec that converts map keys to a slice.
// The map key type must match the slice element type, or slice elements must be any.
// If the destination slice is nil, a new slice is created with appropriate capacity.
// The order of keys corresponds to the iteration order of map keys (not guaranteed).
//
// Example:
//
//	source := map[string]int{"apple": 1, "banana": 2}
//	var dst []string
//	// After conversion: dst contains ["apple", "banana"] (order may vary)
var Keys convcodec.Codec = func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
	srcType := reflectutils.IndirectType(src.Type())
	dstType := reflectutils.IndirectType(dst.Type())

	// Use optimized type checking
	opt := newOptimizer(srcType, dstType)
	if !opt.isMapToSlice {
		next(fieldName, src, dst)
		return
	}

	// skip if the type of slice elements does not match the type of map keys
	if !opt.canConvert(srcType.Key().Kind(), dstType.Elem().Kind()) {
		next(fieldName, src, dst)
		return
	}

	if dst.IsNil() {
		// create a new pointer if dst is nil
		dst.Set(reflect.MakeSlice(reflect.SliceOf(dstType.Elem()), 0, len(src.MapKeys())))
	}

	keys := src.MapKeys()
	for i := range keys {
		dst.Set(reflect.Append(dst, keys[i]))
	}
}
