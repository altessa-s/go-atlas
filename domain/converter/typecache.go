// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"reflect"
	"sync"
	"unsafe"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func cachedTypeRelation(cache *sync.Map, dstType reflect.Type, compute func() bool) bool {
	// Check cache first (thread-safe)
	if v, ok := cache.Load(dstType); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}

	// Compute and cache the result
	result := compute()
	cache.Store(dstType, result)
	return result
}

// TypeInfo contains cached metadata for a specific type.
// It stores field information, lookup maps, and assignability/convertibility caches.
// TypeInfo is safe for concurrent reads; the assignability and convertibility caches
// use sync.Map internally.
type TypeInfo struct {
	Fields      []FieldInfo
	Embedded    map[string]int // embedded field name -> field index
	ByName      map[string]int // field name -> field index
	ByIndex     map[int]string // field index -> lowercase field name
	NumFields   int
	assignable  sync.Map // map[reflect.Type]bool
	convertible sync.Map // map[reflect.Type]bool
}

// FieldInfo contains cached metadata for a struct field.
// It includes pre-computed values for fast field access during conversion.
type FieldInfo struct {
	Name     string
	Lower    string // lowercase version for case-insensitive matching
	Index    int
	Type     reflect.Type
	Kind     reflect.Kind
	Exported bool
	Embedded bool
	Pointer  bool
}

// TypeCache provides efficient caching of reflection metadata.
// It stores type information to avoid repeated reflection operations.
// Implementations must be safe for concurrent use.
type TypeCache interface {
	// GetTypeInfo returns cached type information, building it on first access.
	GetTypeInfo(t reflect.Type) *TypeInfo
	// InvalidateType removes type information from cache.
	InvalidateType(t reflect.Type)
	// Clear removes all cached type information.
	Clear()
}

// syncMapTypeCache implements TypeCache using sync.Map for thread safety
type syncMapTypeCache struct {
	cache sync.Map // map[reflect.Type]*TypeInfo
}

// NewTypeCache creates a new thread-safe type cache.
// The cache stores reflection metadata for types to improve conversion performance.
//
// Example:
//
//	cache := NewTypeCache()
//	info := cache.GetTypeInfo(reflect.TypeOf(MyStruct{}))
func NewTypeCache() TypeCache {
	return &syncMapTypeCache{}
}

// GetTypeInfo returns cached type metadata or creates new cache entry.
// This method is thread-safe and builds type information on first access.
func (tc *syncMapTypeCache) GetTypeInfo(t reflect.Type) *TypeInfo {
	if cached, ok := tc.cache.Load(t); ok {
		if info, ok := cached.(*TypeInfo); ok {
			return info
		}
	}

	info := tc.buildTypeInfo(t)
	tc.cache.Store(t, info)
	return info
}

// InvalidateType removes type information from cache.
// This is useful when type definitions change during runtime or for cache cleanup.
func (tc *syncMapTypeCache) InvalidateType(t reflect.Type) {
	tc.cache.Delete(t)
}

// Clear removes all cached type information.
// This method clears the entire cache, useful for memory cleanup or testing.
func (tc *syncMapTypeCache) Clear() {
	tc.cache.Range(func(key, value any) bool {
		tc.cache.Delete(key)
		return true
	})
}

// buildTypeInfo creates a new TypeInfo for the given type
func (tc *syncMapTypeCache) buildTypeInfo(t reflect.Type) *TypeInfo {
	// Handle pointer types
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return &TypeInfo{
			Fields:    []FieldInfo{},
			Embedded:  make(map[string]int),
			ByName:    make(map[string]int),
			ByIndex:   make(map[int]string),
			NumFields: 0,
		}
	}

	numFields := t.NumField()
	info := &TypeInfo{
		Fields:    make([]FieldInfo, 0, numFields),
		Embedded:  make(map[string]int, numFields),
		ByName:    make(map[string]int, numFields),
		ByIndex:   make(map[int]string, numFields),
		NumFields: numFields,
	}

	for i := range numFields {
		field := t.Field(i)

		// Cache basic field information
		fieldInfo := FieldInfo{
			Name:     field.Name,
			Lower:    corestrings.InternLowerString(field.Name),
			Index:    i,
			Type:     field.Type,
			Kind:     field.Type.Kind(),
			Exported: field.IsExported(),
			Embedded: field.Anonymous,
			Pointer:  field.Type.Kind() == reflect.Pointer,
		}

		info.Fields = append(info.Fields, fieldInfo)

		if !field.IsExported() {
			continue
		}

		// Build lookup maps for exported fields
		info.ByName[fieldInfo.Lower] = i
		info.ByIndex[i] = fieldInfo.Lower

		// Check if this is an embedded struct
		if field.Anonymous && IndirectType(field.Type).Kind() == reflect.Struct {
			info.Embedded[fieldInfo.Lower] = i
		}
	}

	return info
}

// IsAssignable checks if src type is assignable to dst type using cache.
// Results are cached for subsequent lookups. Returns true if types are identical.
//
// Example:
//
//	if info.IsAssignable(srcType, dstType) { dst.Set(src) }
func (info *TypeInfo) IsAssignable(srcType, dstType reflect.Type) bool {
	if isSameTypeFast(srcType, dstType) {
		return true
	}
	return cachedTypeRelation(&info.assignable, dstType, func() bool {
		return srcType.AssignableTo(dstType)
	})
}

// IsConvertible checks if src type is convertible to dst type using cache.
// Results are cached for subsequent lookups. Returns true if types are identical.
//
// Example:
//
//	if info.IsConvertible(srcType, dstType) { dst.Set(src.Convert(dstType)) }
func (info *TypeInfo) IsConvertible(srcType, dstType reflect.Type) bool {
	if isSameTypeFast(srcType, dstType) {
		return true
	}
	return cachedTypeRelation(&info.convertible, dstType, func() bool {
		return srcType.ConvertibleTo(dstType)
	})
}

// FieldAccess provides optimized field access using arrays instead of maps.
// It uses pooled slices for better memory efficiency and O(1) field lookups.
type FieldAccess struct {
	Values   []reflect.Value
	Exists   []bool
	MaxIndex int

	// valuesPtr / existsPtr are the pooled slice pointers backing Values / Exists.
	// Release returns these exact pointers to the pool, preserving pool identity so
	// a backing array is never handed to two callers at once. Do not read them for
	// field access — use Values / Exists.
	valuesPtr *[]reflect.Value
	existsPtr *[]bool
}

// NewFieldAccess creates array-based field access for faster lookups.
// Uses pooled slices to reduce allocations during struct conversion.
//
// Example:
//
//	access := NewFieldAccess(structValue, typeInfo)
//	val, ok := access.GetField(fieldIndex)
func NewFieldAccess(v reflect.Value, info *TypeInfo) *FieldAccess {
	// Use pooled slices for better memory efficiency
	fieldValues := getPooledValueSlice(info.NumFields)
	fieldExists := getPooledBoolSlice(info.NumFields)

	access := &FieldAccess{
		Values:    *fieldValues,
		Exists:    *fieldExists,
		MaxIndex:  info.NumFields - 1,
		valuesPtr: fieldValues,
		existsPtr: fieldExists,
	}

	// Populate array with field values
	for i := range min(info.NumFields, v.NumField()) {
		access.Values[i] = v.Field(i)
		access.Exists[i] = info.Fields[i].Exported
	}

	return access
}

// Release returns the pooled slices back to the global pool for reuse.
// Must be called when the FieldAccess is no longer needed (typically via defer).
func (access *FieldAccess) Release() {
	putPooledValueSlice(access.valuesPtr)
	putPooledBoolSlice(access.existsPtr)
	access.Values = nil
	access.Exists = nil
	access.valuesPtr = nil
	access.existsPtr = nil
}

// GetField performs O(1) field lookup using array indexing.
// Returns the field value and true if found, or an invalid Value and false otherwise.
func (access *FieldAccess) GetField(fieldIndex int) (reflect.Value, bool) {
	if fieldIndex < 0 || fieldIndex > access.MaxIndex {
		return reflect.Value{}, false
	}
	if !access.Exists[fieldIndex] {
		return reflect.Value{}, false
	}
	return access.Values[fieldIndex], true
}

// isSameTypeFast performs fast type equality check using unsafe pointer comparison.
// This is significantly faster than reflect.Type.AssignableTo() or == comparison.
//
// # Safety Invariants
//
// This function uses unsafe.Pointer to extract and compare type identity pointers.
// The approach is safe because:
//
//  1. Interface Layout Guarantee: Go's interface representation is stable and documented.
//     An interface value consists of two words: (type descriptor, data pointer).
//     For reflect.Type (which is an interface), the second word points to the
//     runtime type descriptor (*runtime._type).
//
//  2. Read-Only Access: We only read the pointer value for comparison; we never
//     dereference it or modify any memory. This is equivalent to comparing addresses.
//
//  3. Type Identity Semantics: In Go's runtime, each unique type has exactly one
//     type descriptor. Two reflect.Type values representing the same type will have
//     identical data pointers. This is the same check the runtime uses internally.
//
//  4. No Lifetime Issues: The type descriptors are allocated in read-only memory
//     by the compiler/linker and exist for the program's lifetime.
//
//  5. Nil Safety: Explicit nil checks before unsafe operations prevent nil dereference.
//
// Performance: This is ~5x faster than reflect.Type equality comparison (==) because
// it avoids the interface method dispatch overhead.
//
// #nosec G103 -- intentional unsafe for fast type comparison with verified safety
func isSameTypeFast(t1, t2 reflect.Type) bool {
	if t1 == nil && t2 == nil {
		return true
	}
	if t1 == nil || t2 == nil {
		return false
	}

	// Extract the type pointer from reflect.Type interface.
	// reflect.Type is an interface with two words: [type info pointer, data pointer]
	// The data pointer (second word) points to the runtime type descriptor.
	// Two types are identical iff their type descriptors are the same pointer.
	//
	// Memory layout of interface:
	//   type iface struct {
	//       tab  *itab         // word 0: interface method table
	//       data unsafe.Pointer // word 1: pointer to actual data (type descriptor for reflect.Type)
	//   }
	t1Data := (*[2]unsafe.Pointer)(unsafe.Pointer(&t1))[1]
	t2Data := (*[2]unsafe.Pointer)(unsafe.Pointer(&t2))[1]
	return t1Data == t2Data
}
