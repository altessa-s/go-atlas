// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
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
	index       int
	pathSegment string
	isEmbedded  bool // true if this is an embedded (anonymous) struct field
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
	beforeRoot, afterRoot := beforeVal, afterVal

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
	state := comparison{
		changed:    (*changedPtr)[:0],
		beforeRoot: beforeRoot,
		afterRoot:  afterRoot,
	}
	ft.compareStructsFast(beforeVal, afterVal, "", &state, 0)
	changed := state.changed

	// Make a copy to return (so we can return the pooled slice)
	result := make([]string, len(changed))
	copy(result, changed)

	// Return slice to pool
	clear(changed)
	*changedPtr = changed
	changedFieldsPool.Put(changedPtr)

	return result
}

// comparison is local to one call. Only pairs on the current recursion path
// are tracked, so the same pair reached independently through two fields still
// produces changes for both field paths.
type comparison struct {
	changed []string
	// unlimited lifts the depth limit, for the comparison of a value the limit stopped the walk at.
	unlimited             bool
	beforeRoot, afterRoot reflect.Value

	// The common shallow path stays entirely on the stack. For deeper
	// paths, a map keeps cycle checks bounded by the inline capacity.
	active [8]referencePair
	depth  int
	spill  map[referencePair]struct{}
}

type referencePair struct {
	typ       reflect.Type
	before    unsafe.Pointer
	after     unsafe.Pointer
	beforeLen int
	afterLen  int
}

func referenceOf(before, after reflect.Value) referencePair {
	ref := referencePair{typ: before.Type(), before: before.UnsafePointer(), after: after.UnsafePointer()}
	if before.Kind() == reflect.Slice {
		ref.beforeLen, ref.afterLen = before.Len(), after.Len()
	}
	return ref
}

func (c *comparison) enter(ref referencePair) bool {
	if slices.Contains(c.active[:min(c.depth, len(c.active))], ref) {
		return false
	}
	if c.depth < len(c.active) {
		c.active[c.depth] = ref
	} else {
		if _, ok := c.spill[ref]; ok {
			return false
		}
		if c.spill == nil {
			c.spill = make(map[referencePair]struct{})
		}
		c.spill[ref] = struct{}{}
	}
	c.depth++
	return true
}

func (c *comparison) leave(ref referencePair) {
	c.depth--
	if c.depth >= len(c.active) {
		delete(c.spill, ref)
	}
}

// isRoot recognizes a pointer back edge to the two input structs.
func (c *comparison) isRoot(ref referencePair) bool {
	return ref.typ == c.beforeRoot.Type() && ref.typ == c.afterRoot.Type() &&
		ref.before == c.beforeRoot.UnsafePointer() && ref.after == c.afterRoot.UnsafePointer()
}

func (ft *Tracker) depthExceeded(state *comparison, depth int) bool {
	return !state.unlimited && ft.opts.maxDepth > 0 && depth > ft.opts.maxDepth
}

// equalBeyondLimit decides whether two values the depth limit stops the walk at differ, with the same field selection as the walk
// itself (promoted fields of embedded structs, json:"-" and ignored fields): they are compared with the limit lifted and the
// result is collapsed to the enclosing path by the caller.
func (ft *Tracker) equalBeyondLimit(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	sub := comparison{beforeRoot: state.beforeRoot, afterRoot: state.afterRoot, unlimited: true}
	return ft.fastCompare(before, after, prefix, &sub, depth) && len(sub.changed) == 0
}

// compareStructsFast uses optimized comparison with zero-allocation path building.
func (ft *Tracker) compareStructsFast(before, after reflect.Value, prefix string, state *comparison, depth int) {
	typ := before.Type()

	// Get or build type info
	info := ft.getTypeInfo(typ)
	if !info.trackable {
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
		// Build path efficiently using buffer
		pathBuf = pathBuf[:0]
		if prefix != "" {
			pathBuf = append(pathBuf, prefix...)
			pathBuf = append(pathBuf, '.')
		}
		pathBuf = append(pathBuf, field.pathSegment...)

		// Skip ignored fields; the string(pathBuf) map key does not allocate.
		if _, ok := ft.opts.ignoreFields[string(pathBuf)]; ok {
			continue
		}

		beforeField := before.Field(field.index)
		afterField := after.Field(field.index)

		if field.isEmbedded {
			// Flatten child paths, but report the embedded field itself when
			// its nil state or opaque value changes.
			if !ft.fastCompare(beforeField, afterField, prefix, state, depth+1) {
				state.changed = append(state.changed, fieldPathOf(prefix, field.pathSegment, pathBuf))
			}
			continue
		}

		// Compare primitives before materializing the path, so equal fields
		// cost no allocation. Top-level fields reuse the cached JSON name.
		if eq, ok := primitiveEqual(beforeField, afterField); ok {
			if !eq {
				state.changed = append(state.changed, fieldPathOf(prefix, field.pathSegment, pathBuf))
			}
			continue
		}

		fieldPath := fieldPathOf(prefix, field.pathSegment, pathBuf)
		if !ft.fastCompare(beforeField, afterField, fieldPath, state, depth+1) {
			state.changed = append(state.changed, fieldPath)
		}
	}
}

// fieldPathOf returns the reported path of a struct field: the cached JSON
// name at the top level, otherwise a copy of the "prefix.name" bytes in buf.
func fieldPathOf(prefix, jsonName string, buf []byte) string {
	if prefix == "" {
		return jsonName
	}
	return string(buf)
}

// fastCompare uses type-specific fast paths for common cases
func (ft *Tracker) fastCompare(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	// Handle nil values
	if !before.IsValid() && !after.IsValid() {
		return true
	}
	if !before.IsValid() || !after.IsValid() {
		return false
	}

	// Interfaces are transparent. Checking the concrete types here catches a
	// type change at the current path before traversing either value.
	if before.Kind() == reflect.Interface {
		if before.IsNil() || after.IsNil() {
			return before.IsNil() == after.IsNil()
		}
		before, after = before.Elem(), after.Elem()
		if before.Type() != after.Type() {
			return false
		}
	}

	if eq, ok := primitiveEqual(before, after); ok {
		return eq
	}

	switch before.Kind() {
	case reflect.Pointer:
		return ft.comparePointer(before, after, prefix, state, depth)

	case reflect.Struct:
		info := ft.getTypeInfo(before.Type())
		if info.trackable {
			if ft.depthExceeded(state, depth) {
				return ft.equalBeyondLimit(before, after, prefix, state, depth)
			}
			ft.compareStructsFast(before, after, prefix, state, depth)
			return true
		}
		// An unexported embedded struct may expose public children, but
		// if it has none its private state is not part of the diff.
		return valuesEqual(before, after)

	case reflect.Slice, reflect.Map:
		return ft.compareReferencedCollection(before, after, prefix, state, depth)

	case reflect.Array:
		if ft.depthExceeded(state, depth) {
			return ft.equalBeyondLimit(before, after, prefix, state, depth)
		}
		return ft.compareSliceFast(before, after, prefix, state, depth)

	default:
		return valuesEqual(before, after)
	}
}

func (ft *Tracker) comparePointer(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	if before.IsNil() || after.IsNil() {
		return before.IsNil() == after.IsNil()
	}
	ref := referenceOf(before, after)
	if ref.before == ref.after || state.isRoot(ref) || !state.enter(ref) {
		return true
	}
	defer state.leave(ref)
	before, after = before.Elem(), after.Elem()
	// A single pointer has the depth of its field or collection slot.
	// Additional indirection within the slot also consumes depth.
	if before.Kind() == reflect.Pointer || before.Kind() == reflect.Interface {
		if ft.depthExceeded(state, depth) {
			return ft.equalBeyondLimit(before, after, prefix, state, depth)
		}
		depth++
	}
	return ft.fastCompare(before, after, prefix, state, depth)
}

func (ft *Tracker) compareReferencedCollection(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	if before.Len() == 0 && after.Len() == 0 {
		return true
	}
	// Beyond the depth limit the collection is not traversed: it is one value, equal or changed as a whole, so the enclosing path
	// is reported instead of silently dropping a difference.
	if ft.depthExceeded(state, depth) {
		return ft.equalBeyondLimit(before, after, prefix, state, depth)
	}
	ref := referenceOf(before, after)
	if (ref.before == ref.after && ref.beforeLen == ref.afterLen) || !state.enter(ref) {
		return true
	}
	defer state.leave(ref)

	if before.Kind() == reflect.Map {
		return ft.compareMapFast(before, after, prefix, state, depth)
	}
	return ft.compareSliceFast(before, after, prefix, state, depth)
}

// compareSliceFast optimized slice comparison
func (ft *Tracker) compareSliceFast(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	if before.Len() != after.Len() {
		if _, ok := ft.opts.ignoreFields[prefix]; !ok {
			state.changed = append(state.changed, prefix)
		}
		return true
	}

	if before.Len() == 0 {
		return true
	}

	// Use stack-allocated buffer for small indices
	var indexBuf [IndexBufferSize]byte
	buf := indexBuf[:0]

	// buildPath materializes the "prefix[i]" path string. It is called lazily —
	// only for elements that actually need a reported path.
	buildPath := func(i int) string {
		buf = strconv.AppendInt(buf[:0], int64(i), 10)

		itemPath := make([]byte, 0, len(prefix)+len(buf)+2)
		itemPath = append(itemPath, prefix...)
		itemPath = append(itemPath, '[')
		itemPath = append(itemPath, buf...)
		itemPath = append(itemPath, ']')
		return string(itemPath)
	}

	for i := range before.Len() {
		beforeItem := before.Index(i)
		afterItem := after.Index(i)

		// Compare primitive elements before building their path: on the common
		// all-equal diff the loop is then allocation-free instead of paying two
		// path allocations per element.
		if eq, ok := primitiveEqual(beforeItem, afterItem); ok {
			if !eq {
				if pathStr := buildPath(i); !ft.isIgnored(pathStr) {
					state.changed = append(state.changed, pathStr)
				}
			}
			continue
		}

		pathStr := buildPath(i)
		if ft.isIgnored(pathStr) {
			continue
		}
		if !ft.fastCompare(beforeItem, afterItem, pathStr, state, depth+1) {
			state.changed = append(state.changed, pathStr)
		}
	}
	return true
}

// isNilReference reports whether v is a nil pointer, interface, map, slice, func or channel; user methods are never called on it.
func isNilReference(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return false
	}
}

// valuesEqual compares two values as opaque wholes: with their Equal(T) bool method when the type has one (time.Time, decimals, whose
// equal values need not be deeply equal), otherwise with reflect.DeepEqual. Values reached through unexported fields are not part of
// the diff and compare equal.
func valuesEqual(before, after reflect.Value) bool {
	if !before.CanInterface() || !after.CanInterface() {
		return true
	}
	if isNilReference(before) || isNilReference(after) {
		return reflect.DeepEqual(before.Interface(), after.Interface())
	}
	if m := before.MethodByName("Equal"); m.IsValid() {
		if mt := m.Type(); mt.NumIn() == 1 && mt.In(0) == after.Type() && mt.NumOut() == 1 && mt.Out(0).Kind() == reflect.Bool {
			return m.Call([]reflect.Value{after})[0].Bool()
		}
	}
	return reflect.DeepEqual(before.Interface(), after.Interface())
}

// isIgnored reports whether path is in the configured ignore set.
func (ft *Tracker) isIgnored(path string) bool {
	_, ok := ft.opts.ignoreFields[path]
	return ok
}

// primitiveEqual reports whether both values are of a directly comparable
// primitive kind (ok) and, if so, whether they are equal.
func primitiveEqual(before, after reflect.Value) (equal, ok bool) {
	switch before.Kind() {
	case reflect.String:
		return before.String() == after.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return before.Int() == after.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return before.Uint() == after.Uint(), true
	case reflect.Float32, reflect.Float64:
		return before.Float() == after.Float(), true
	case reflect.Bool:
		return before.Bool() == after.Bool(), true
	default:
		return false, false
	}
}

// compareMapFast optimized map comparison
func (ft *Tracker) compareMapFast(before, after reflect.Value, prefix string, state *comparison, depth int) bool {
	if before.Len() == 0 && after.Len() == 0 {
		return true
	}

	// Reuse one key/value pair across iterations instead of the per-entry
	// copies MapIter.Key and MapIter.Value allocate.
	mapType := before.Type()
	key := reflect.New(mapType.Key()).Elem()
	beforeVal := reflect.New(mapType.Elem()).Elem()
	for iter := before.MapRange(); iter.Next(); {
		key.SetIterKey(iter)
		beforeVal.SetIterValue(iter)
		afterVal := after.MapIndex(key)

		if !afterVal.IsValid() {
			// Key present in "before" but missing in "after"
			mapPath := prefix + "[" + formatMapKey(key) + "]"
			if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
				state.changed = append(state.changed, mapPath)
			}
			continue
		}

		// Compare primitives before building the path: equal entries then
		// cost no allocation.
		if eq, ok := primitiveEqual(beforeVal, afterVal); ok {
			if !eq {
				mapPath := prefix + "[" + formatMapKey(key) + "]"
				if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
					state.changed = append(state.changed, mapPath)
				}
			}
			continue
		}

		mapPath := prefix + "[" + formatMapKey(key) + "]"
		if ft.isIgnored(mapPath) {
			continue
		}
		if !ft.fastCompare(beforeVal, afterVal, mapPath, state, depth+1) {
			state.changed = append(state.changed, mapPath)
		}
	}

	// Also detect keys present in "after" but missing in "before".
	for iter := after.MapRange(); iter.Next(); {
		key.SetIterKey(iter)
		if before.MapIndex(key).IsValid() {
			continue
		}

		mapPath := prefix + "[" + formatMapKey(key) + "]"
		if _, ok := ft.opts.ignoreFields[mapPath]; !ok {
			state.changed = append(state.changed, mapPath)
		}
	}

	return true
}

// buildTypeInfoFast builds type information with optimized field name extraction
// Handles embedded (anonymous) struct fields by flattening their fields into the parent
func (ft *Tracker) buildTypeInfoFast(typ reflect.Type) *typeInfo {
	info := &typeInfo{
		fields: make([]fieldInfo, 0, typ.NumField()),
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		embedded := field.Anonymous &&
			(field.Type.Kind() == reflect.Struct ||
				(field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct))
		if !field.IsExported() && !embedded {
			continue
		}

		// Even if every public field is ignored, compare the struct by its
		// tracked fields rather than falling back to DeepEqual.
		info.trackable = true
		tagName, _, _ := strings.Cut(field.Tag.Get(ft.opts.tagName), ",")
		if tagName == "-" {
			continue
		}
		info.fields = append(info.fields, fieldInfo{
			index:       i,
			pathSegment: escapeFieldSegment(ft.getFieldNameFast(field)),
			isEmbedded:  embedded && tagName == "",
		})
	}

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

	result, _, _ := strings.Cut(tagVal, ",")
	if result == "" {
		result = toSnakeCaseFast(field.Name)
	}

	// Store in cache
	ft.fieldNameCache.Store(cacheKey, result)
	return result
}

// toSnakeCaseFast preserves the historical ASCII convention of separating every
// uppercase letter (UserID -> user_i_d), unlike corestrings.ToSnakeCase, which
// groups acronyms. Unicode names use the shared Unicode-aware converter.
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
			if r >= utf8.RuneSelf {
				return corestrings.ToSnakeCase(s)
			}
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
		if r >= utf8.RuneSelf {
			return corestrings.ToSnakeCase(s)
		}
		result = coreslices.AppendIf(result, i > 0 && r >= 'A' && r <= 'Z', '_')
		if r >= 'A' && r <= 'Z' {
			result = append(result, byte(r+asciiUpperToLower))
		} else {
			result = append(result, byte(r))
		}
	}
	return string(result)
}
