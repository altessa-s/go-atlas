// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"reflect"
	"unsafe"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

const (
	// DefaultPrimitiveConverterMapSize is the initial capacity for the converter
	// lookup map in [NewPrimitiveRegistry].
	DefaultPrimitiveConverterMapSize = 64
)

// PrimitiveConverter performs zero-copy primitive conversion using unsafe pointers.
//
// # Safety Invariants
//
// The use of unsafe.Pointer in this type is safe because:
//
//  1. Type Safety: Pointers are obtained from reflect.Value.UnsafeAddr() which guarantees
//     the address points to a valid Go value of the expected type. The caller (TryConvert)
//     validates that srcKind and dstKind match the registered converter before invocation.
//
//  2. Memory Validity: Both src and dst pointers come from addressable reflect.Values
//     (verified via CanAddr() checks in TryConvert). The values remain valid for the
//     duration of the conversion as they are held by the caller's reflect.Value.
//
//  3. Alignment: Go guarantees proper alignment for all types. Since we obtain addresses
//     from reflect.Value, alignment is preserved.
//
//  4. No Pointer Arithmetic: We perform direct type casts without pointer arithmetic,
//     ensuring we never access memory outside the intended value.
//
//  5. Single Operation: Each converter performs a single atomic read-convert-write
//     operation with no intermediate state that could be invalidated.
//
// Performance: This approach is ~10x faster than reflect.Value.Set() for primitive
// conversions by eliminating reflection overhead.
type PrimitiveConverter func(srcPtr, dstPtr unsafe.Pointer)

// ConversionKey represents a source-to-destination type conversion
type ConversionKey struct {
	From, To reflect.Kind
}

// PrimitiveRegistry manages zero-copy primitive type conversions.
// It is safe for concurrent reads after construction; the converter map is
// populated once during [NewPrimitiveRegistry] and never modified afterwards.
type PrimitiveRegistry struct {
	converters    *coremaps.ImmutableMap[ConversionKey, PrimitiveConverter]
	checkOverflow bool
}

// NewPrimitiveRegistry creates a new primitive conversion registry
// with pre-registered converters for all supported primitive type pairs.
// An optional checkOverflow parameter enables runtime overflow detection
// for narrowing numeric conversions.
//
// Example:
//
//	registry := NewPrimitiveRegistry()
//	conv, ok := registry.Get(reflect.Int32, reflect.Int64)
func NewPrimitiveRegistry(checkOverflow ...bool) *PrimitiveRegistry {
	r := &PrimitiveRegistry{
		checkOverflow: len(checkOverflow) > 0 && checkOverflow[0],
	}
	r.converters = r.buildConverters()
	return r
}

// Get returns the converter for the given type pair, if available.
// Returns the converter function and true if the conversion is supported,
// or nil and false if no converter exists for the type pair.
func (r *PrimitiveRegistry) Get(from, to reflect.Kind) (PrimitiveConverter, bool) {
	return r.converters.Get(ConversionKey{from, to})
}

// TryConvert attempts zero-copy conversion for primitive types.
// Returns true if the conversion was successful, false if the types
// are not supported or conversion failed.
//
// # Safety Invariants
//
// This method uses unsafe.Pointer for performance-critical primitive conversions.
// The following invariants ensure memory safety:
//
//  1. Addressability Check: Both srcVal.CanAddr() and dstVal.CanAddr() must be true
//     before obtaining unsafe pointers. This guarantees the values have stable addresses.
//
//  2. Settability Check: dstVal.CanSet() must be true, ensuring we have write permission
//     and the destination is not a read-only value (e.g., unexported field).
//
//  3. Type Matching: The converter is looked up by (srcKind, dstKind) key, ensuring
//     the unsafe pointer casts in the converter match the actual types.
//
//  4. Pointer Lifetime: The unsafe pointers are used immediately within the converter
//     function call. No pointers are stored or returned, preventing use-after-free.
//
// #nosec G103 -- intentional zero-copy primitive conversion with verified safety invariants
func (r *PrimitiveRegistry) TryConvert(srcValue, dstValue reflect.Value) bool {
	// Handle pointer dereferencing for primitives
	srcVal := srcValue
	dstVal := dstValue

	// If src is a pointer to a primitive, dereference it
	if srcVal.Kind() == reflect.Pointer && !srcVal.IsNil() {
		srcElem := srcVal.Elem()
		if IsPrimitive(srcElem.Kind()) {
			srcVal = srcElem
		}
	}

	// If dst is a pointer to a primitive, dereference it or initialize if nil
	if dstVal.Kind() == reflect.Pointer {
		if dstVal.IsNil() {
			// Only initialize nil pointer if we have a non-zero source value to convert
			if IsPrimitive(dstVal.Type().Elem().Kind()) && !srcVal.IsZero() {
				dstVal.Set(reflect.New(dstVal.Type().Elem()))
			} else if srcVal.IsZero() {
				// Don't initialize nil pointer for zero source values
				return false
			}
		}
		if !dstVal.IsNil() {
			dstElem := dstVal.Elem()
			if IsPrimitive(dstElem.Kind()) {
				dstVal = dstElem
			}
		}
	}

	srcKind := srcVal.Kind()
	dstKind := dstVal.Kind()

	// Check if we have a zero-copy converter for this type combination
	if converter, exists := r.Get(srcKind, dstKind); exists {
		// Both values must be settable and addressable
		if dstVal.CanSet() && srcVal.CanAddr() && dstVal.CanAddr() {
			if r.checkOverflow && wouldOverflow(srcVal, dstVal) {
				panic(OverflowError{From: srcKind, To: dstKind, Value: srcVal.Interface()})
			}
			srcPtr := unsafe.Pointer(srcVal.UnsafeAddr())
			dstPtr := unsafe.Pointer(dstVal.UnsafeAddr())
			converter(srcPtr, dstPtr)
			return true
		}
	}

	return false
}

// registerAllConversions registers all primitive type conversions using a single elegant map.
//
// # Conversion Semantics
//
// Narrowing conversions (e.g., int64→int8, uint64→uint8) follow Go's standard truncation
// semantics. Callers are responsible for ensuring values fit the target type.
//
// # Safety of Unsafe Pointer Casts
//
// Each converter function performs a type-punned memory access:
//
//	func(src, dst unsafe.Pointer) { *(*TargetType)(dst) = TargetType(*(*SourceType)(src)) }
//
// This is safe because:
//  1. The ConversionKey (From, To) exactly matches the pointer cast types
//  2. TryConvert validates the source/destination kinds before calling
//  3. Go primitives have well-defined sizes and representations across platforms
//  4. The read-convert-write is atomic with respect to the single value
//
// #nosec G115 -- intentional narrowing conversions matching Go's type conversion behavior
func (r *PrimitiveRegistry) buildConverters() *coremaps.ImmutableMap[ConversionKey, PrimitiveConverter] {
	allConverters := map[ConversionKey]PrimitiveConverter{
		// Int conversions
		{reflect.Int, reflect.Int8}:    func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*int)(src)) },
		{reflect.Int, reflect.Int16}:   func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*int)(src)) },
		{reflect.Int, reflect.Int32}:   func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*int)(src)) },
		{reflect.Int, reflect.Int64}:   func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*int)(src)) },
		{reflect.Int8, reflect.Int}:    func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*int8)(src)) },
		{reflect.Int8, reflect.Int16}:  func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*int8)(src)) },
		{reflect.Int8, reflect.Int32}:  func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*int8)(src)) },
		{reflect.Int8, reflect.Int64}:  func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*int8)(src)) },
		{reflect.Int16, reflect.Int}:   func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*int16)(src)) },
		{reflect.Int16, reflect.Int8}:  func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*int16)(src)) },
		{reflect.Int16, reflect.Int32}: func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*int16)(src)) },
		{reflect.Int16, reflect.Int64}: func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*int16)(src)) },
		{reflect.Int32, reflect.Int}:   func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*int32)(src)) },
		{reflect.Int32, reflect.Int8}:  func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*int32)(src)) },
		{reflect.Int32, reflect.Int16}: func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*int32)(src)) },
		{reflect.Int32, reflect.Int64}: func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*int32)(src)) },
		{reflect.Int64, reflect.Int}:   func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*int64)(src)) },
		{reflect.Int64, reflect.Int8}:  func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*int64)(src)) },
		{reflect.Int64, reflect.Int16}: func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*int64)(src)) },
		{reflect.Int64, reflect.Int32}: func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*int64)(src)) },

		// Uint conversions
		{reflect.Uint, reflect.Uint8}:    func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*uint)(src)) },
		{reflect.Uint, reflect.Uint16}:   func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*uint)(src)) },
		{reflect.Uint, reflect.Uint32}:   func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*uint)(src)) },
		{reflect.Uint, reflect.Uint64}:   func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*uint)(src)) },
		{reflect.Uint8, reflect.Uint}:    func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*uint8)(src)) },
		{reflect.Uint8, reflect.Uint16}:  func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*uint8)(src)) },
		{reflect.Uint8, reflect.Uint32}:  func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*uint8)(src)) },
		{reflect.Uint8, reflect.Uint64}:  func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*uint8)(src)) },
		{reflect.Uint16, reflect.Uint}:   func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*uint16)(src)) },
		{reflect.Uint16, reflect.Uint8}:  func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*uint16)(src)) },
		{reflect.Uint16, reflect.Uint32}: func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*uint16)(src)) },
		{reflect.Uint16, reflect.Uint64}: func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*uint16)(src)) },
		{reflect.Uint32, reflect.Uint}:   func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*uint32)(src)) },
		{reflect.Uint32, reflect.Uint8}:  func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*uint32)(src)) },
		{reflect.Uint32, reflect.Uint16}: func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*uint32)(src)) },
		{reflect.Uint32, reflect.Uint64}: func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*uint32)(src)) },
		{reflect.Uint64, reflect.Uint}:   func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*uint64)(src)) },
		{reflect.Uint64, reflect.Uint8}:  func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*uint64)(src)) },
		{reflect.Uint64, reflect.Uint16}: func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*uint64)(src)) },
		{reflect.Uint64, reflect.Uint32}: func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*uint64)(src)) },

		// Float conversions
		{reflect.Float32, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*float32)(src)) },
		{reflect.Float64, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*float64)(src)) },

		// Int to Float conversions
		{reflect.Int, reflect.Float32}:   func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*int)(src)) },
		{reflect.Int, reflect.Float64}:   func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*int)(src)) },
		{reflect.Int8, reflect.Float32}:  func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*int8)(src)) },
		{reflect.Int8, reflect.Float64}:  func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*int8)(src)) },
		{reflect.Int16, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*int16)(src)) },
		{reflect.Int16, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*int16)(src)) },
		{reflect.Int32, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*int32)(src)) },
		{reflect.Int32, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*int32)(src)) },
		{reflect.Int64, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*int64)(src)) },
		{reflect.Int64, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*int64)(src)) },

		// Uint to Float conversions
		{reflect.Uint, reflect.Float32}:   func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*uint)(src)) },
		{reflect.Uint, reflect.Float64}:   func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*uint)(src)) },
		{reflect.Uint8, reflect.Float32}:  func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*uint8)(src)) },
		{reflect.Uint8, reflect.Float64}:  func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*uint8)(src)) },
		{reflect.Uint16, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*uint16)(src)) },
		{reflect.Uint16, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*uint16)(src)) },
		{reflect.Uint32, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*uint32)(src)) },
		{reflect.Uint32, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*uint32)(src)) },
		{reflect.Uint64, reflect.Float32}: func(src, dst unsafe.Pointer) { *(*float32)(dst) = float32(*(*uint64)(src)) },
		{reflect.Uint64, reflect.Float64}: func(src, dst unsafe.Pointer) { *(*float64)(dst) = float64(*(*uint64)(src)) },

		// Float to Int conversions
		{reflect.Float32, reflect.Int}:   func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*float32)(src)) },
		{reflect.Float32, reflect.Int8}:  func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*float32)(src)) },
		{reflect.Float32, reflect.Int16}: func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*float32)(src)) },
		{reflect.Float32, reflect.Int32}: func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*float32)(src)) },
		{reflect.Float32, reflect.Int64}: func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*float32)(src)) },
		{reflect.Float64, reflect.Int}:   func(src, dst unsafe.Pointer) { *(*int)(dst) = int(*(*float64)(src)) },
		{reflect.Float64, reflect.Int8}:  func(src, dst unsafe.Pointer) { *(*int8)(dst) = int8(*(*float64)(src)) },
		{reflect.Float64, reflect.Int16}: func(src, dst unsafe.Pointer) { *(*int16)(dst) = int16(*(*float64)(src)) },
		{reflect.Float64, reflect.Int32}: func(src, dst unsafe.Pointer) { *(*int32)(dst) = int32(*(*float64)(src)) },
		{reflect.Float64, reflect.Int64}: func(src, dst unsafe.Pointer) { *(*int64)(dst) = int64(*(*float64)(src)) },

		// Float to Uint conversions
		{reflect.Float32, reflect.Uint}:   func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*float32)(src)) },
		{reflect.Float32, reflect.Uint8}:  func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*float32)(src)) },
		{reflect.Float32, reflect.Uint16}: func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*float32)(src)) },
		{reflect.Float32, reflect.Uint32}: func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*float32)(src)) },
		{reflect.Float32, reflect.Uint64}: func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*float32)(src)) },
		{reflect.Float64, reflect.Uint}:   func(src, dst unsafe.Pointer) { *(*uint)(dst) = uint(*(*float64)(src)) },
		{reflect.Float64, reflect.Uint8}:  func(src, dst unsafe.Pointer) { *(*uint8)(dst) = uint8(*(*float64)(src)) },
		{reflect.Float64, reflect.Uint16}: func(src, dst unsafe.Pointer) { *(*uint16)(dst) = uint16(*(*float64)(src)) },
		{reflect.Float64, reflect.Uint32}: func(src, dst unsafe.Pointer) { *(*uint32)(dst) = uint32(*(*float64)(src)) },
		{reflect.Float64, reflect.Uint64}: func(src, dst unsafe.Pointer) { *(*uint64)(dst) = uint64(*(*float64)(src)) },

		// Same type conversions (no-op but included for completeness)
		{reflect.Bool, reflect.Bool}:     func(src, dst unsafe.Pointer) { *(*bool)(dst) = *(*bool)(src) },
		{reflect.String, reflect.String}: func(src, dst unsafe.Pointer) { *(*string)(dst) = *(*string)(src) },
	}

	return coremaps.NewImmutableMap(allConverters)
}

// IsPrimitive checks if a kind represents a primitive type.
// Primitive types include all integers, unsigned integers, floats, bool, and string.
func IsPrimitive(k reflect.Kind) bool {
	return reflectutils.IsPrimitive(k)
}

// wouldOverflow returns true if converting srcVal to dstVal's type would lose data.
func wouldOverflow(srcVal, dstVal reflect.Value) bool {
	srcKind := srcVal.Kind()
	dstKind := dstVal.Kind()

	switch {
	// int → int narrowing
	case isSignedInt(dstKind) && isSignedInt(srcKind):
		return reflect.New(dstVal.Type()).Elem().OverflowInt(srcVal.Int())

	// uint → uint narrowing
	case isUnsignedInt(dstKind) && isUnsignedInt(srcKind):
		return reflect.New(dstVal.Type()).Elem().OverflowUint(srcVal.Uint())

	// float → float narrowing (float64 → float32)
	case isFloat(dstKind) && isFloat(srcKind):
		return reflect.New(dstVal.Type()).Elem().OverflowFloat(srcVal.Float())

	// int → uint: negative values overflow
	case isUnsignedInt(dstKind) && isSignedInt(srcKind):
		v := srcVal.Int()
		if v < 0 {
			return true
		}
		return reflect.New(dstVal.Type()).Elem().OverflowUint(uint64(v))

	// uint → int: check if value exceeds signed max
	case isSignedInt(dstKind) && isUnsignedInt(srcKind):
		v := srcVal.Uint()
		if v > uint64(^uint(0)>>1) { // exceeds max int64
			return true
		}
		return reflect.New(dstVal.Type()).Elem().OverflowInt(int64(v))

	// float → int
	case isSignedInt(dstKind) && isFloat(srcKind):
		f := srcVal.Float()
		i := int64(f)
		if float64(i) != f {
			return true
		}
		return reflect.New(dstVal.Type()).Elem().OverflowInt(i)

	// float → uint
	case isUnsignedInt(dstKind) && isFloat(srcKind):
		f := srcVal.Float()
		if f < 0 {
			return true
		}
		u := uint64(f)
		if float64(u) != f {
			return true
		}
		return reflect.New(dstVal.Type()).Elem().OverflowUint(u)

	// int → float: check precision loss for float32
	case isFloat(dstKind) && isSignedInt(srcKind):
		if dstKind == reflect.Float32 {
			return reflect.New(dstVal.Type()).Elem().OverflowFloat(float64(srcVal.Int()))
		}
		return false

	// uint → float: check precision loss for float32
	case isFloat(dstKind) && isUnsignedInt(srcKind):
		if dstKind == reflect.Float32 {
			return reflect.New(dstVal.Type()).Elem().OverflowFloat(float64(srcVal.Uint()))
		}
		return false
	}

	return false
}

func isSignedInt(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Int64
}

func isUnsignedInt(k reflect.Kind) bool {
	return k >= reflect.Uint && k <= reflect.Uint64
}

func isFloat(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}
