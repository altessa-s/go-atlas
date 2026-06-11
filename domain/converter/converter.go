// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"fmt"
	"iter"
	"reflect"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/domain/converter/codec"

	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

const (
	// EmptySliceLength is the initial length for destination slices created during
	// slice conversion.
	EmptySliceLength = 0
	// SingleElementSliceCapacity is the capacity hint when converting a single
	// struct into a one-element slice.
	SingleElementSliceCapacity = 1
	// MaxStackAllocFields is the upper bound on source struct fields for which
	// [Converter] uses a stack-allocated fast path instead of pooled field access.
	MaxStackAllocFields = 4
)

// Converter provides high-performance struct-to-struct conversion with various optimizations.
// It supports field mapping, filtering, embedded structs, slices, maps, and protocol buffers.
// The converter caches type information and uses object pooling for optimal performance.
//
// A Converter is safe for concurrent use after construction; do not modify options
// after calling [New].
type Converter[T ConversionSource, U ConversionDestination] struct {
	opts *options
	// Clean abstractions for performance optimization
	typeCache         TypeCache
	primitiveRegistry *PrimitiveRegistry
	// convertByKindHandler caches the bound method value passed as the
	// codec chain's terminal handler. Taking a method value (conv.convertByKind)
	// allocates a small closure each time, so caching it once at construction
	// time eliminates an allocation per field for codec-using converters that
	// walk many fields. Initialized by every constructor below; never nil
	// after construction.
	convertByKindHandler convcodec.CodecHandler
}

// Shared global resources reused by one-shot Convert() calls to avoid
// re-creating expensive type caches and primitive registries.
var (
	sharedTypeCache         = NewTypeCache()
	sharedPrimitiveRegistry = NewPrimitiveRegistry()
)

// Convert converts src to dst.
// T must be a valid conversion source (struct, pointer to struct, or slice of structs).
// U must be a valid conversion destination (pointer to struct or pointer to slice).
// Note: type compatibility is validated at runtime and panics on invalid combinations.
//
// Example:
//
//	type User struct { Name string; Age int }
//	type UserDTO struct { Name string; Age int }
//
//	user := User{Name: "John", Age: 30}
//	var dto UserDTO
//	Convert(user, &dto) // dto now contains user data
func Convert[T ConversionSource, U ConversionDestination](src T, dst U, opt ...Option) U {
	o := defaultOptions().apply(opt...)
	conv := &Converter[T, U]{
		opts:              o,
		typeCache:         sharedTypeCache,
		primitiveRegistry: sharedPrimitiveRegistry,
	}
	if o.overflowCheck {
		conv.primitiveRegistry = NewPrimitiveRegistry(true)
	}
	conv.convertByKindHandler = conv.convertByKind
	conv.Convert(src, dst)
	return dst
}

// New creates a new Converter with the specified options.
// The converter is initialized with default settings that can be customized
// using functional options. Performance optimizations include type caching,
// string interning, primitive registries, and object pooling.
//
// Example:
//
//	conv := New[User, UserDTO](
//		WithIgnoreFields("password"),
//		WithFieldMappings(map[string]string{"user_name": "name"}),
//	)
//	conv.Convert(user, &dto)
func New[T ConversionSource, U ConversionDestination](opt ...Option) *Converter[T, U] {
	o := defaultOptions().apply(opt...)
	conv := &Converter[T, U]{
		opts:              o,
		typeCache:         NewTypeCache(),
		primitiveRegistry: NewPrimitiveRegistry(o.overflowCheck),
	}
	conv.convertByKindHandler = conv.convertByKind

	return conv
}

// NewShared creates a new Converter with the specified options using shared global caches.
// This is significantly more efficient than New() for repeated use as it reuses
// type information and primitive registries.
//
// Example:
//
//	conv := NewShared[User, UserDTO]()
//	conv.Convert(user, &dto)
func NewShared[T ConversionSource, U ConversionDestination](opt ...Option) *Converter[T, U] {
	o := defaultOptions().apply(opt...)
	conv := &Converter[T, U]{
		opts:              o,
		typeCache:         sharedTypeCache,
		primitiveRegistry: sharedPrimitiveRegistry,
	}
	conv.convertByKindHandler = conv.convertByKind

	if o.overflowCheck {
		// If overflow check is needed, we might need a specific registry or just use the shared one
		// implementation detail: sharedPrimitiveRegistry might not have overflow check enabled by default
		// but checking NewPrimitiveRegistry implementation would be good.
		// For now, let's assume if overflow check is requested, we might need a new registry
		// OR we can rely on the fact that sharedPrimitiveRegistry handles it?
		// Line 70 in Convert creates a new one if overflowCheck is true.
		conv.primitiveRegistry = NewPrimitiveRegistry(true)
	}

	return conv
}

// ConvertSeq returns a lazy iterator that converts each element of src to type U on demand.
// Elements are converted one at a time as the iterator is consumed, avoiding a full
// destination slice allocation up front.
func ConvertSeq[T any, U any](src []T, opt ...Option) iter.Seq[U] {
	conv := New[T, U](opt...)
	return conv.ConvertSeq(src)
}

// ConvertMapSeq returns a lazy iterator that converts each key-value pair of src
// to destination types K2 and V2 on demand. Keys and values are converted independently.
func ConvertMapSeq[K1 comparable, V1 any, K2 comparable, V2 any](src map[K1]V1, opt ...Option) iter.Seq2[K2, V2] {
	conv := New[any, any](opt...)
	return func(yield func(K2, V2) bool) {
		for k, v := range src {
			var dk K2
			var dv V2
			conv.convertValue("", reflect.ValueOf(k), reflect.ValueOf(&dk).Elem())
			conv.convertValue("", reflect.ValueOf(v), reflect.ValueOf(&dv).Elem())
			if !yield(dk, dv) {
				return
			}
		}
	}
}

// NewAny creates a new non-generic Converter.
// This function allows existing code to work without type parameters.
// For better type safety, use New[T, U]() with explicit type parameters.
//
// Example:
//
//	conv := NewAny(
//		WithIgnoreFields("internal"),
//	)
//	conv.Convert(sourceStruct, &destStruct)
func NewAny(opt ...Option) *Converter[any, any] {
	return New[any, any](opt...)
}

// Convert converts src to dst using the configured converter options.
// The conversion handles structs, slices of structs, and embedded structures
// based on the converter's configuration. Panics if src/dst kinds are incompatible.
//
// Example:
//
//	conv := New[User, *UserDTO]()
//	conv.Convert(user, &dto)
func (conv *Converter[T, U]) Convert(src T, dst U) {
	// Nil source produces no conversion — dst remains untouched.
	srcRaw := reflect.ValueOf(src)
	if !srcRaw.IsValid() || (srcRaw.Kind() == reflect.Pointer && srcRaw.IsNil()) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(OverflowError); ok {
				panic(r)
			}
			panic(CompileTimeTypeError{
				Source:      src,
				Destination: dst,
				Reason:      "Type combination not supported by converter",
				Original:    r,
			})
		}
	}()

	srcValue := reflect.Indirect(srcRaw)
	srcKind := srcValue.Kind()

	// src must be a struct, map, slice or array
	if srcKind != reflect.Struct && srcKind != reflect.Slice {
		panic("src must be a struct or slice of structs")
	}

	dstValue := reflect.ValueOf(dst)

	// dst must be a pointer to a struct or slice of structs
	if (srcKind == reflect.Struct && (dstValue.Kind() != reflect.Pointer ||
		reflect.Indirect(dstValue).Kind() != reflect.Struct)) ||
		reflect.Indirect(dstValue).Kind() != srcKind {
		panic("dst must be the same kind as src")
	}

	if srcKind == reflect.Struct {
		conv.convertStruct(srcValue, dstValue)
		return
	}

	srcType := IndirectType(srcValue.Type())

	if IndirectType(srcType.Elem()).Kind() != reflect.Struct {
		panic("src slice must be a slice of structs")
	}

	dstType := IndirectType(dstValue.Type())

	if dstValue.Kind() != reflect.Pointer || dstType.Kind() != reflect.Slice ||
		IndirectType(dstType.Elem()).Kind() != reflect.Struct {
		panic("dst slice must be a pointer to slice of structs")
	}

	conv.convertValue("", srcValue, reflect.Indirect(dstValue))
}

// ConvertSeq returns a lazy iterator that converts each element of src to type U.
// Uses the receiver's cached type information and options for each conversion.
func (conv *Converter[T, U]) ConvertSeq(src []T) iter.Seq[U] {
	return func(yield func(U) bool) {
		for _, item := range src {
			var dst U
			dstValue := reflect.ValueOf(&dst)
			conv.convertValue("", reflect.ValueOf(item), dstValue.Elem())
			if !yield(dst) {
				return
			}
		}
	}
}

// ConvertMapSeq returns a lazy iterator that converts each key-value pair of the source map.
// dstKeyType and dstValueType specify the target types; if nil, the source key or value type
// is used as-is (useful when the generic type parameter is 'any').
func (conv *Converter[T, U]) ConvertMapSeq(src any, dstKeyType reflect.Type, dstValueType reflect.Type) iter.Seq2[any, any] {
	srcValue := reflect.ValueOf(src)
	if srcValue.Kind() != reflect.Map {
		return func(yield func(any, any) bool) {}
	}

	return func(yield func(any, any) bool) {
		for _, key := range srcValue.MapKeys() {
			srcMapValue := srcValue.MapIndex(key)

			dk := dstKeyType
			if dk == nil {
				dk = key.Type()
			}
			dv := dstValueType
			if dv == nil {
				dv = srcMapValue.Type()
			}

			dstKey := conv.convertMapKey("", key, dk)
			dstVal := conv.convertMapValue("", key, srcMapValue, dv)

			if !yield(dstKey.Interface(), dstVal.Interface()) {
				return
			}
		}
	}
}

// convertSmallStruct converts small structs using stack allocation to reduce heap allocations.
// Uses direct dstValue.Field() via ByName map instead of pooled FieldAccess slices.
func (conv *Converter[T, U]) convertSmallStruct(src reflect.Value, dst reflect.Value) bool {
	dstValue := reflect.Indirect(dst)

	// Only optimize for small structs
	if src.NumField() > MaxStackAllocFields {
		return false
	}

	// Get cached data for both source and destination
	srcInfo := conv.typeCache.GetTypeInfo(src.Type())
	dstInfo := conv.typeCache.GetTypeInfo(dstValue.Type())

	// Process source fields using stack allocation and cached data
	for i := range src.NumField() {
		// Skip unexported fields using cached data
		if i >= len(srcInfo.Fields) || !srcInfo.Fields[i].Exported {
			continue
		}

		srcFieldValue := src.Field(i)

		// Skip if src is pointer to nil
		if srcFieldValue.Kind() == reflect.Pointer && srcFieldValue.IsNil() {
			continue
		}

		// Skip embedded structs using cached data
		if srcInfo.Fields[i].Embedded {
			return false
		}

		// Use pre-computed lowercase field name from cache
		srcFieldName := srcInfo.Fields[i].Lower

		// Check if field mapping exists, use mapped name if available
		dstFieldName := srcFieldName
		if len(conv.opts.fieldMappings) > 0 {
			if mappedName, exists := conv.opts.fieldMappings[srcFieldName]; exists {
				dstFieldName = mappedName
			}
		}

		// Direct field lookup via cached ByName map — no pooled slices needed
		dstFieldIndex, exists := dstInfo.ByName[dstFieldName]
		if !exists {
			continue
		}

		dstFieldValue := dstValue.Field(dstFieldIndex)
		if !dstFieldValue.IsValid() || !dstFieldValue.CanSet() {
			continue
		}

		conv.convertValue(srcFieldName, srcFieldValue, dstFieldValue)
	}

	return true
}

// convertStruct converts one struct to another.
func (conv *Converter[T, U]) convertStruct(src reflect.Value, dst reflect.Value) {
	dstValue := reflect.Indirect(dst)

	// if dst has embedded structs, iterate through them
	if conv.opts.handleEmbeddedStructs {
		dstInfo := conv.typeCache.GetTypeInfo(dstValue.Type())
		for fieldName, fieldIndex := range dstInfo.Embedded {
			dstEmbeddedFieldValue := dstValue.Field(fieldIndex)
			if dstEmbeddedFieldValue.Kind() == reflect.Pointer && dstEmbeddedFieldValue.IsNil() {
				if conv.opts.alwaysInitializeEmbeddedStruct {
					dstEmbeddedFieldValue.Set(reflect.New(dstEmbeddedFieldValue.Type().Elem()))
				} else {
					// create a new pointer if dst is nil
					tmpVal := reflect.New(dstEmbeddedFieldValue.Type().Elem())
					conv.convertValue("", src, tmpVal)

					// if the embedded struct has non-zero values, set it
					if !isStructZero(tmpVal.Elem()) {
						dstEmbeddedFieldValue.Set(tmpVal)
					}
				}
				continue
			}
			conv.convertValue(fieldName, src, dstEmbeddedFieldValue)
		}
	}

	// Try stack allocation optimization for small structs
	if src.NumField() <= MaxStackAllocFields && !conv.opts.handleEmbeddedStructs {
		if conv.convertSmallStruct(src, dst) {
			return // Successfully processed with stack allocation
		}
	}

	// Get cached data for source and destination
	srcInfo := conv.typeCache.GetTypeInfo(src.Type())
	dstInfo := conv.typeCache.GetTypeInfo(dstValue.Type())

	// Use array-based field access for destination (O(1) lookups)
	dstFieldAccess := NewFieldAccess(dstValue, dstInfo)
	defer dstFieldAccess.Release()

	for i := range src.NumField() {
		// Skip unexported fields using cached data
		if i >= len(srcInfo.Fields) || !srcInfo.Fields[i].Exported {
			continue
		}

		srcFieldValue := src.Field(i)

		// skip if src is pointer to nil.
		if srcFieldValue.Kind() == reflect.Pointer &&
			srcFieldValue.IsNil() {
			continue
		}

		// Use pre-computed lowercase field name from cache
		srcFieldName := srcInfo.Fields[i].Lower

		// Check if field mapping exists, use mapped name if available
		dstFieldName := srcFieldName
		if len(conv.opts.fieldMappings) > 0 {
			if mappedName, exists := conv.opts.fieldMappings[srcFieldName]; exists {
				dstFieldName = mappedName
			}
		}

		// O(1) array-based lookup instead of map lookup
		var dstFieldValue reflect.Value
		var dstFieldExists bool

		// Find destination field index using cached mapping
		if dstFieldIndex, exists := dstInfo.ByName[dstFieldName]; exists {
			dstFieldValue, dstFieldExists = dstFieldAccess.GetField(dstFieldIndex)
		}

		// embedded struct using cached data
		if srcInfo.Fields[i].Embedded {
			// if dst does not have the field, try to unwrap the embedded struct
			if dstFieldExists {
				if dstFieldValue.Kind() == reflect.Pointer && dstFieldValue.IsNil() {
					// create a new pointer if dst is nil
					dstFieldValue.Set(reflect.New(dstFieldValue.Type().Elem()))
				}
				dst = dstFieldValue
			}

			conv.convertStruct(reflect.Indirect(srcFieldValue), dst)
			continue
		}

		if !dstFieldExists || !dstFieldValue.IsValid() || !dstFieldValue.CanSet() {
			continue
		}

		conv.convertValue(srcFieldName, srcFieldValue, dstFieldValue)
	}
}

// convertSlices converts source slice to destination slice with element type conversion.
func (conv *Converter[T, U]) convertSlices(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	if dstValue.Kind() == reflect.Pointer {
		if dstValue.IsNil() {
			dstValue.Set(reflect.New(dstValue.Type().Elem()))
		}
		dstValue = dstValue.Elem()
	}

	dstValue.Set(reflect.MakeSlice(dstValue.Type(), EmptySliceLength, srcValue.Len()))

	for i := range srcValue.Len() {
		dstVal := reflect.New(IndirectType(dstValue.Type().Elem()))

		conv.convertValue(makeFieldName(fieldName, strconv.Itoa(i)), srcValue.Index(i), dstVal.Elem())

		if dstValue.Type().Elem().Kind() != reflect.Pointer {
			dstValue.Set(reflect.Append(dstValue, dstVal.Elem()))
		} else {
			dstValue.Set(reflect.Append(dstValue, dstVal))
		}
	}
}

// convertMaps converts source map to destination map with key/value type conversion.
func (conv *Converter[T, U]) convertMaps(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	if dstValue.Kind() == reflect.Pointer {
		if dstValue.IsNil() {
			dstValue.Set(reflect.New(dstValue.Type().Elem()))
		}
		dstValue = dstValue.Elem()
	}

	// Initialize destination map if needed
	if srcValue.Len() > 0 {
		dstValue.Set(reflect.MakeMapWithSize(dstValue.Type(), srcValue.Len()))
	} else {
		dstValue.Set(reflect.MakeMap(dstValue.Type()))
	}

	dstKeyType := dstValue.Type().Key()
	dstValueType := dstValue.Type().Elem()

	for _, key := range srcValue.MapKeys() {
		srcMapValue := srcValue.MapIndex(key)

		// Convert key using helper function
		dstKey := conv.convertMapKey(fieldName, key, dstKeyType)

		// Convert value using helper function
		dstMapValue := conv.convertMapValue(fieldName, key, srcMapValue, dstValueType)

		// Set the converted key-value pair in destination map
		dstValue.SetMapIndex(dstKey, dstMapValue)
	}
}

// convertMapKey converts map key with fallback chain.
func (conv *Converter[T, U]) convertMapKey(fieldName string, key reflect.Value, dstKeyType reflect.Type) reflect.Value {
	switch {
	case key.Type().AssignableTo(dstKeyType):
		return key
	case key.Type().ConvertibleTo(dstKeyType):
		return key.Convert(dstKeyType)
	default:
		dstKey := reflect.New(dstKeyType).Elem()
		conv.convertValue(makeFieldName(fieldName, "key"), key, dstKey)
		return dstKey
	}
}

// convertMapValue converts map value handling both struct and primitive types.
func (conv *Converter[T, U]) convertMapValue(fieldName string, key reflect.Value, srcMapValue reflect.Value, dstValueType reflect.Type) reflect.Value {
	valueFieldName := makeFieldName(fieldName, fmt.Sprintf("[%v]", key.Interface()))

	if IndirectType(dstValueType).Kind() == reflect.Struct {
		return conv.convertStructMapValue(valueFieldName, srcMapValue, dstValueType)
	}
	return conv.convertPrimitiveMapValue(valueFieldName, srcMapValue, dstValueType)
}

// convertStructMapValue handles conversion of struct values in maps.
func (conv *Converter[T, U]) convertStructMapValue(fieldName string, srcMapValue reflect.Value, dstValueType reflect.Type) reflect.Value {
	dstMapValue := reflect.New(IndirectType(dstValueType))
	conv.convertValue(fieldName, srcMapValue, dstMapValue.Elem())

	if dstValueType.Kind() == reflect.Pointer {
		return dstMapValue
	}
	return dstMapValue.Elem()
}

// convertPrimitiveMapValue handles conversion of primitive values in maps.
func (conv *Converter[T, U]) convertPrimitiveMapValue(fieldName string, srcMapValue reflect.Value, dstValueType reflect.Type) reflect.Value {
	dstMapValue := reflect.New(IndirectType(dstValueType)).Elem()

	switch {
	case srcMapValue.Type().AssignableTo(dstValueType):
		dstMapValue.Set(srcMapValue)
	case srcMapValue.Type().ConvertibleTo(dstValueType):
		dstMapValue.Set(srcMapValue.Convert(dstValueType))
	default:
		conv.convertValue(fieldName, srcMapValue, dstMapValue)
	}

	return dstMapValue
}

func (conv *Converter[T, U]) convertValue(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	// Optimize nil/zero checks - combine conditions to reduce branches
	if conv.opts.ignoreNilValues || conv.opts.ignoreZeroValues {
		srcValueKind := srcValue.Kind()
		isNilValue := (srcValueKind == reflect.Pointer || srcValueKind == reflect.Slice || srcValueKind == reflect.Map) && srcValue.IsNil()
		if (conv.opts.ignoreNilValues && isNilValue) || (conv.opts.ignoreZeroValues && srcValue.IsZero()) {
			return
		}
	}

	// Honor ignore fields early, including fast paths (assignable/primitive).
	if len(conv.opts.ignoreFields) > 0 {
		if _, ok := conv.opts.ignoreFields[fieldName]; ok {
			return
		}
	}

	// Branch prediction optimization: reorder checks based on likelihood
	// Most likely path: direct type assignability (90%+ of cases)
	if conv.isTypeCachedAssignable(srcValue.Type(), dstValue.Type()) {
		dstValue.Set(srcValue)
		return
	}

	// When codecs are registered, route the entire dispatch through the
	// codec chain so a codec-handled type whose Go kind is struct/slice/map
	// is consulted before the built-in field-by-field copy below — otherwise
	// the kind-dispatch shadows the codec and silently zeros codec-handled
	// types like time.Time. The primitive fast-path is skipped on this
	// branch because codecs may also intercept primitives (e.g. an enum-int
	// codec); convertByKind reaches the same primitive path as terminal.
	if conv.opts.codecsSet.HasCodecs() {
		conv.opts.codecsSet.Run(fieldName, srcValue, dstValue, conv.convertByKindHandler)
		return
	}

	// Fast path: zero-copy primitive conversion (common for numeric fields).
	// Only safe when no codecs are registered.
	if conv.primitiveRegistry.TryConvert(srcValue, dstValue) {
		return
	}

	conv.convertByKind(fieldName, srcValue, dstValue)
}

// convertByKind performs the built-in conversion dispatched on the source and
// destination reflect kinds (struct, slice, map), falling back to a convertible
// scalar conversion. It is used directly on the codec-free path and as the
// terminal handler of the codec chain, so it must not invoke codecs itself.
func (conv *Converter[T, U]) convertByKind(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	// Pre-compute type information once
	srcType := srcValue.Type()
	dstType := dstValue.Type()
	srcValueType := IndirectType(srcType)
	dstValueType := IndirectType(dstType)
	srcKind := srcValueType.Kind()
	dstKind := dstValueType.Kind()

	// Branch prediction optimization: most common conversion types first
	// Struct-to-struct conversion (most common case ~70%)
	if srcKind == reflect.Struct {
		if dstKind == reflect.Struct {
			// Common case: both structs - optimize pointer handling
			srcIsNil := srcValue.Kind() == reflect.Pointer && srcValue.IsNil()
			// Partial-update merge: a present-but-empty nested struct clears the destination instead of recursing.
			if conv.opts.updateMerge && !srcIsNil &&
				isUpdateStructEmpty(reflect.Indirect(srcValue)) {
				dstValue.Set(reflect.Zero(dstValue.Type()))
				return
			}
			if dstValue.Kind() == reflect.Pointer && dstValue.IsNil() && !srcIsNil {
				dstValue.Set(reflect.New(dstValueType))
			}
			conv.convertStruct(reflect.Indirect(srcValue), dstValue)
			return
		}
		// Less common: struct to slice conversion
		if dstKind == reflect.Slice && isSameTypeFast(IndirectType(dstValueType.Elem()), srcValueType) {
			conv.convertStructToSlice(fieldName, srcValue, dstValue)
			return
		}
	}

	// Slice-to-slice conversion (second most common ~20%)
	if srcKind == reflect.Slice && dstKind == reflect.Slice {
		conv.convertSlices(fieldName, srcValue, dstValue)
		return
	}

	// Map-to-map conversion (less common ~10%)
	if srcKind == reflect.Map && dstKind == reflect.Map {
		conv.convertMaps(fieldName, srcValue, dstValue)
		return
	}

	// Fallback to convertible scalar conversion (least common cases ~5%)
	conv.convertConvertible(fieldName, srcValue, dstValue)
}

// convertStructToSlice converts a struct to a slice containing that struct.
func (conv *Converter[T, U]) convertStructToSlice(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	dstValue.Set(reflect.MakeSlice(reflect.SliceOf(IndirectType(dstValue.Type()).Elem()), EmptySliceLength, SingleElementSliceCapacity))

	dstVal := reflect.New(IndirectType(dstValue.Type().Elem()))
	conv.convertValue(fieldName, srcValue, dstVal.Elem())

	if dstValue.Type().Elem().Kind() != reflect.Pointer {
		dstValue.Set(reflect.Append(dstValue, dstVal.Elem()))
	} else {
		dstValue.Set(reflect.Append(dstValue, dstVal))
	}
}

// convertConvertible performs a direct convertible-type conversion (e.g. between
// defined types that share an underlying type) when no codec or structural
// conversion applies. It honors the overflow-check option for narrowing numeric
// conversions. This is the terminal step of the built-in dispatch and never
// invokes codecs.
func (conv *Converter[T, U]) convertConvertible(fieldName string, srcValue reflect.Value, dstValue reflect.Value) {
	dst := dstValue
	if dst.Kind() == reflect.Pointer && dst.IsNil() {
		dst.Set(reflect.New(dst.Type().Elem()))
		dst = dst.Elem()
	}

	dst = reflect.Indirect(dst)

	dstType := IndirectType(dst.Type())

	if !conv.isTypeCachedConvertible(IndirectType(srcValue.Type()), dstType) {
		return
	}

	srcIndirect := reflect.Indirect(srcValue)
	srcKind := srcIndirect.Kind()
	dstKind := dst.Kind()
	if conv.opts.overflowCheck && IsPrimitive(srcKind) && IsPrimitive(dstKind) {
		if wouldOverflow(srcIndirect, dst) {
			panic(OverflowError{Field: fieldName, From: srcKind, To: dstKind, Value: srcIndirect.Interface()})
		}
	}
	dst.Set(srcIndirect.Convert(dstType))
}

// isTypeCachedAssignable checks if src type is assignable to dst type using cache
func (conv *Converter[T, U]) isTypeCachedAssignable(srcType, dstType reflect.Type) bool {
	// Get type info from cache and check assignability
	srcInfo := conv.typeCache.GetTypeInfo(srcType)
	return srcInfo.IsAssignable(srcType, dstType)
}

// isTypeCachedConvertible checks if src type is convertible to dst type using cache
func (conv *Converter[T, U]) isTypeCachedConvertible(srcType, dstType reflect.Type) bool {
	// Get type info from cache and check convertibility
	srcInfo := conv.typeCache.GetTypeInfo(srcType)
	return srcInfo.IsConvertible(srcType, dstType)
}

// isStructZero returns true if all fields of the struct are zero values.
func isStructZero(v reflect.Value) bool {
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.IsZero() {
			return false
		}
	}
	return true
}

// isUpdateStructEmpty reports whether v carries no update instructions: every
// pointer/slice/map/interface field is nil and every other field is its zero
// value. Used by [WithUpdateMerge] to detect a present-but-empty nested update
// struct, which signals "clear the whole field" rather than "merge nothing".
func isUpdateStructEmpty(v reflect.Value) bool {
	for i := range v.NumField() {
		field := v.Field(i)
		switch field.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
			if !field.IsNil() {
				return false
			}
		default:
			if !field.IsZero() {
				return false
			}
		}
	}
	return true
}

func makeFieldName(n ...string) string {
	if len(n) == 0 {
		return ""
	}
	if len(n) == 1 {
		return n[0]
	}

	strs := make([]string, 0, len(n))
	for i := range len(n) {
		if n[i] != "" {
			strs = append(strs, n[i])
		}
	}
	return strings.Join(strs, ".")
}

// IndirectType returns the underlying type after dereferencing all pointer levels.
// If the type is not a pointer, it returns the type unchanged.
//
// Example:
//
//	t := IndirectType(reflect.TypeOf(&User{})) // returns reflect.Type of User
func IndirectType(t reflect.Type) reflect.Type {
	return reflectutils.IndirectType(t)
}
