// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const (
	// SmallBufferThreshold is the prefix-length threshold that determines whether
	// a path-building buffer is drawn from the small or medium pool.
	SmallBufferThreshold = 64
	// ShortStringLength is the upper bound on input length for which
	// toSnakeCaseFast uses a stack-allocated byte buffer.
	ShortStringLength = 10
	// StackBufferSize is the size of the stack-allocated buffer used by
	// toSnakeCaseFast for short field names.
	StackBufferSize = 20
	// IndexBufferSize is the stack-allocated buffer size for formatting slice
	// indices in path strings.
	IndexBufferSize = 32
	// InitialSliceCapacity is the starting capacity for the pooled changed-fields
	// slice returned by [Tracker.GetChangedFields].
	InitialSliceCapacity = 8
	// SmallBufferCapacity is the pooled byte-slice capacity for path building
	// when the prefix is shorter than [SmallBufferThreshold].
	SmallBufferCapacity = 64
	// MediumBufferCapacity is the pooled byte-slice capacity for path building
	// when the prefix is [SmallBufferThreshold] or longer.
	MediumBufferCapacity = 256
)

const (
	// asciiUpperToLower is the offset to convert ASCII uppercase to lowercase.
	asciiUpperToLower = 32
)

// typeInfo caches reflection information for a struct type
type typeInfo struct {
	fields    []fieldInfo
	trackable bool
}

// fieldInfo caches information about a struct field
type fieldInfo struct {
	index      int
	name       string
	jsonName   string
	isExported bool
	isEmbedded bool // true if this is an embedded (anonymous) struct field
}

// Pre-allocated buffer pools for different sizes
var (
	smallBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, SmallBufferCapacity)
			return &b
		},
	}

	mediumBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, MediumBufferCapacity)
			return &b
		},
	}

	// Pool for changed fields slices
	changedFieldsPool = sync.Pool{
		New: func() any {
			s := make([]string, 0, InitialSliceCapacity)
			return &s
		},
	}
)

// Tracker provides optimized field tracking with minimal allocations.
// It is safe for concurrent use; type and field-name caches use sync.Map internally.
type Tracker struct {
	typeCache      sync.Map
	fieldNameCache sync.Map
	opts           *options
}

// NewTracker creates a new optimized tracker instance.
// Use functional options to customize the tracker behavior.
//
// Example:
//
//	tracker := NewTracker(
//	    WithIgnoreFields("updated_at", "etag"),
//	    WithMaxDepth(5),
//	    WithTagName("json"),
//	)
func NewTracker(opts ...Option) *Tracker {
	o := newOptions(opts...)
	return &Tracker{
		opts: o,
	}
}

// GetChangedFields compares two struct instances and returns a list of field paths that differ.
// Field paths use dot notation for nested structures (e.g., "address.city").
// Returns an empty slice if both values are equal or if comparison fails.
//
// This is a convenience function that creates a new Tracker for one-time comparisons.
// For frequent comparisons, create a Tracker instance with NewTracker and reuse it
// to benefit from caching (10-20% performance improvement).
//
// Example:
//
//	before := &Person{FirstName: "John", Email: "old@example.com"}
//	after := &Person{FirstName: "Jane", Email: "old@example.com"}
//	changed := GetChangedFields(before, after)
//	// Returns: ["first_name"]
//
// With options:
//
//	changed := GetChangedFields(before, after,
//	    WithIgnoreFields("updated_at", "etag"),
//	    WithMaxDepth(5),
//	)
func GetChangedFields(before, after any, opts ...Option) []string {
	tracker := NewTracker(opts...)
	return tracker.GetChangedFields(before, after)
}

// GetChangedFields tracks changes with instance-level caching.
// This method benefits from type and field name caching when the same Tracker
// instance is reused across multiple comparisons.
func (ft *Tracker) GetChangedFields(before, after any) []string {
	if before == nil || after == nil {
		return []string{}
	}

	beforeVal := reflect.ValueOf(before)
	afterVal := reflect.ValueOf(after)

	// Dereference pointers
	if beforeVal.Kind() == reflect.Pointer {
		if beforeVal.IsNil() {
			return []string{}
		}
		beforeVal = beforeVal.Elem()
	}

	if afterVal.Kind() == reflect.Pointer {
		if afterVal.IsNil() {
			return []string{}
		}
		afterVal = afterVal.Elem()
	}

	// Both must be structs
	if beforeVal.Kind() != reflect.Struct || afterVal.Kind() != reflect.Struct {
		return []string{}
	}

	// Types must match
	if beforeVal.Type() != afterVal.Type() {
		return []string{}
	}

	// Get pooled slice for changed fields
	changedPtr := changedFieldsPool.Get().(*[]string) //nolint:errcheck
	changed := *changedPtr
	changed = changed[:0] // Reset length but keep capacity

	ft.compareStructsFast(beforeVal, afterVal, "", &changed, 0)

	// Make a copy to return (so we can return the pooled slice)
	result := make([]string, len(changed))
	copy(result, changed)

	// Return slice to pool
	*changedPtr = changed
	changedFieldsPool.Put(changedPtr)

	return result
}

// compareStructsFast uses optimized comparison with zero-allocation path building
func (ft *Tracker) compareStructsFast(before, after reflect.Value, prefix string, changed *[]string, depth int) {
	if ft.opts.maxDepth > 0 && depth > ft.opts.maxDepth {
		return
	}

	typ := before.Type()

	// Get or build type info
	info := ft.getTypeInfo(typ)
	if info == nil || !info.trackable {
		return
	}

	// Use pre-allocated buffer for path building
	var pathBuf []byte
	var pathBufPtr *[]byte
	if len(prefix) < SmallBufferThreshold {
		pathBufPtr = smallBufferPool.Get().(*[]byte) //nolint:errcheck
		pathBuf = *pathBufPtr
		defer func() {
			*pathBufPtr = pathBuf[:0]
			smallBufferPool.Put(pathBufPtr)
		}()
	} else {
		pathBufPtr = mediumBufferPool.Get().(*[]byte) //nolint:errcheck
		pathBuf = *pathBufPtr
		defer func() {
			*pathBufPtr = pathBuf[:0]
			mediumBufferPool.Put(pathBufPtr)
		}()
	}

	for _, field := range info.fields {
		// Handle embedded struct fields - recursively process their fields
		if field.isEmbedded {
			beforeField := before.Field(field.index)
			afterField := after.Field(field.index)

			// Recursively compare the embedded fields with the current prefix.
			// This flattens the embedded fields into the parent without adding the embedded struct name.
			//
			// Note: embedded fields may be either a struct or a pointer-to-struct. Using fastCompare
			// ensures pointers are handled safely (nil checks + dereference).
			_ = ft.fastCompare(beforeField, afterField, prefix, changed, depth+1)
			continue
		}

		if !field.isExported || field.jsonName == "-" {
			continue
		}

		// Build path efficiently using buffer
		pathBuf = pathBuf[:0]
		if prefix != "" {
			pathBuf = append(pathBuf, prefix...)
			pathBuf = append(pathBuf, '.')
		}
		pathBuf = append(pathBuf, field.jsonName...)
		fieldPath := string(pathBuf)

		// Skip ignored fields
		if _, ok := ft.opts.ignoreFields[fieldPath]; ok {
			continue
		}

		beforeField := before.Field(field.index)
		afterField := after.Field(field.index)

		// Fast path for common types
		if !ft.fastCompare(beforeField, afterField, fieldPath, changed, depth+1) {
			*changed = append(*changed, fieldPath)
		}
	}
}

// fastCompare uses type-specific fast paths for common cases
func (ft *Tracker) fastCompare(before, after reflect.Value, prefix string, changed *[]string, depth int) bool {
	// Handle nil values
	if !before.IsValid() && !after.IsValid() {
		return true
	}
	if !before.IsValid() || !after.IsValid() {
		return false
	}

	kind := before.Kind()

	// Fast paths for common primitive types
	switch kind {
	case reflect.String:
		return before.String() == after.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return before.Int() == after.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return before.Uint() == after.Uint()
	case reflect.Float32, reflect.Float64:
		return before.Float() == after.Float()
	case reflect.Bool:
		return before.Bool() == after.Bool()
	case reflect.Pointer:
		// Handle pointer comparison
		if before.IsNil() && after.IsNil() {
			return true
		}
		if before.IsNil() || after.IsNil() {
			return false
		}

		// Dereference and compare
		beforeElem := before.Elem()
		afterElem := after.Elem()

		if beforeElem.Kind() == reflect.Struct {
			info := ft.getTypeInfo(beforeElem.Type())
			if info != nil && info.trackable {
				ft.compareStructsFast(beforeElem, afterElem, prefix, changed, depth)
				return true
			}
			return reflect.DeepEqual(beforeElem.Interface(), afterElem.Interface())
		}

		return ft.fastCompare(beforeElem, afterElem, prefix, changed, depth)

	case reflect.Struct:
		info := ft.getTypeInfo(before.Type())
		if info != nil && info.trackable {
			ft.compareStructsFast(before, after, prefix, changed, depth)
			return true
		}
		return reflect.DeepEqual(before.Interface(), after.Interface())

	case reflect.Slice, reflect.Array:
		return ft.compareSliceFast(before, after, prefix, changed, depth)

	case reflect.Map:
		return ft.compareMapFast(before, after, prefix, changed, depth)

	case reflect.Interface:
		if before.IsNil() && after.IsNil() {
			return true
		}
		if before.IsNil() || after.IsNil() {
			return false
		}
		return ft.fastCompare(before.Elem(), after.Elem(), prefix, changed, depth)

	default:
		// Fallback to DeepEqual for complex types
		return reflect.DeepEqual(before.Interface(), after.Interface())
	}
}

// compareSliceFast optimized slice comparison
func (ft *Tracker) compareSliceFast(before, after reflect.Value, prefix string, changed *[]string, depth int) bool {
	if before.Len() != after.Len() {
		if _, ok := ft.opts.ignoreFields[prefix]; !ok {
			*changed = append(*changed, prefix)
		}
		return true
	}

	if before.Len() == 0 {
		return true
	}

	// Use stack-allocated buffer for small indices
	var indexBuf [IndexBufferSize]byte
	buf := indexBuf[:0]

	for i := range before.Len() {
		// Build path with index
		buf = strconv.AppendInt(buf[:0], int64(i), 10)

		itemPath := make([]byte, 0, len(prefix)+len(buf)+2)
		itemPath = append(itemPath, prefix...)
		itemPath = append(itemPath, '[')
		itemPath = append(itemPath, buf...)
		itemPath = append(itemPath, ']')

		beforeItem := before.Index(i)
		afterItem := after.Index(i)

		pathStr := string(itemPath)
		if beforeItem.Kind() == reflect.Struct {
			ft.compareStructsFast(beforeItem, afterItem, pathStr, changed, depth)
		} else if !ft.fastCompare(beforeItem, afterItem, pathStr, changed, depth) {
			if _, ok := ft.opts.ignoreFields[pathStr]; !ok {
				*changed = append(*changed, pathStr)
			}
		}
	}
	return true
}

// compareMapFast optimized map comparison
func (ft *Tracker) compareMapFast(before, after reflect.Value, prefix string, changed *[]string, depth int) bool {
	if before.Len() == 0 && after.Len() == 0 {
		return true
	}

	beforeKeys := before.MapKeys()
	for _, key := range beforeKeys {
		beforeVal := before.MapIndex(key)
		afterVal := after.MapIndex(key)

		if !afterVal.IsValid() {
			// Key present in "before" but missing in "after"
			mapPath := prefix + "[" + formatMapKey(key) + "]"
			if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
				*changed = append(*changed, mapPath)
			}
			continue
		}

		// Optimize for string keys (most common)
		var mapPath string
		if key.Kind() == reflect.String {
			keyStr := key.String()
			mapPath = prefix + "[" + keyStr + "]"
		} else {
			mapPath = prefix + "[" + formatMapKey(key) + "]"
		}

		if !ft.fastCompare(beforeVal, afterVal, mapPath, changed, depth) {
			if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
				*changed = append(*changed, mapPath)
			}
		}
	}

	// Also detect keys present in "after" but missing in "before".
	afterKeys := after.MapKeys()
	for _, key := range afterKeys {
		beforeVal := before.MapIndex(key)
		if beforeVal.IsValid() {
			continue
		}

		mapPath := prefix + "[" + formatMapKey(key) + "]"
		if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
			*changed = append(*changed, mapPath)
		}
	}

	return true
}

// formatMapKey formats a map key efficiently
func formatMapKey(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.String:
		return v.String()
	default:
		// Fallback - this allocates but is rare
		return v.String()
	}
}

// buildTypeInfoFast builds type information with optimized field name extraction
// Handles embedded (anonymous) struct fields by flattening their fields into the parent
func (ft *Tracker) buildTypeInfoFast(typ reflect.Type) *typeInfo {
	info := &typeInfo{
		fields: make([]fieldInfo, 0, typ.NumField()),
	}

	trackable := false

	for i := range typ.NumField() {
		field := typ.Field(i)

		// Check if this is an embedded struct field
		if field.Anonymous &&
			(field.Type.Kind() == reflect.Struct ||
				(field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct)) {
			// This is an embedded struct - flatten its fields
			fInfo := fieldInfo{
				index:      i,
				name:       field.Name,
				isExported: field.IsExported(),
				isEmbedded: true,
			}
			info.fields = append(info.fields, fInfo)
			trackable = true
		} else {
			// Regular field
			fInfo := fieldInfo{
				index:      i,
				name:       field.Name,
				isExported: field.IsExported(),
				isEmbedded: false,
			}

			if fInfo.isExported {
				fInfo.jsonName = ft.getFieldNameFast(field)
				if fInfo.jsonName != "-" {
					trackable = true
				}
			}

			info.fields = append(info.fields, fInfo)
		}
	}

	info.trackable = trackable

	return info
}

func (ft *Tracker) getTypeInfo(typ reflect.Type) *typeInfo {
	if cached, ok := ft.typeCache.Load(typ); ok {
		return cached.(*typeInfo) //nolint:errcheck
	}

	info := ft.buildTypeInfoFast(typ)
	ft.typeCache.Store(typ, info)
	return info
}

// getFieldNameFast extracts field name with instance-level caching
func (ft *Tracker) getFieldNameFast(field reflect.StructField) string {
	tagVal := field.Tag.Get(ft.opts.tagName)
	cacheKey := field.Name + ":" + ft.opts.tagName + ":" + tagVal

	// Check cache
	if cached, ok := ft.fieldNameCache.Load(cacheKey); ok {
		return cached.(string) //nolint:errcheck
	}

	var result string
	if tagVal != "" {
		// Optimize: avoid Split for simple cases
		commaIdx := strings.IndexByte(tagVal, ',')
		if commaIdx == -1 {
			if tagVal != "" {
				result = tagVal
			} else {
				result = toSnakeCaseFast(field.Name)
			}
		} else {
			if commaIdx > 0 {
				result = tagVal[:commaIdx]
			} else {
				result = toSnakeCaseFast(field.Name)
			}
		}
	} else {
		result = toSnakeCaseFast(field.Name)
	}

	// Store in cache
	ft.fieldNameCache.Store(cacheKey, result)
	return result
}

// toSnakeCaseFast converts PascalCase to snake_case with minimal allocations
func toSnakeCaseFast(s string) string {
	if s == "" {
		return ""
	}

	// Fast path for common short names
	// #nosec G602 -- StackBufferSize(20) > ShortStringLength*2-1(19), bounds are safe
	if len(s) <= ShortStringLength {
		var buf [StackBufferSize]byte // Stack allocated for short strings
		n := 0
		for i, r := range s {
			if i > 0 && r >= 'A' && r <= 'Z' {
				buf[n] = '_'
				n++
			}
			if r >= 'A' && r <= 'Z' {
				buf[n] = byte(r + asciiUpperToLower)
			} else {
				buf[n] = byte(r)
			}
			n++
		}
		return string(buf[:n])
	}

	// For longer strings, use heap allocation
	result := make([]byte, 0, len(s)*2)
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		if r >= 'A' && r <= 'Z' {
			result = append(result, byte(r+asciiUpperToLower))
		} else {
			result = append(result, byte(r))
		}
	}
	return string(result)
}
